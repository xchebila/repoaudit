# ADR 0008 — Plugin System : subprocess uniquement, octets seuls, dégradation sur échec

## Statut

Accepté (2026-07-23). Précédé d'un audit explicite de l'interface `core.Analyzer` existante avant tout code — cette phase diffère des précédentes en ce qu'elle ouvre la porte à du code hors de ce repo, donc des hypothèses implicites sans conséquence en interne (tout est revu/testé/mergé avec la même discipline) deviennent des questions de sécurité une fois ce code écrit par un tiers non audité.

## Constat de l'audit : une interface Go n'est pas une frontière de sécurité

Vérifié dans le code existant avant de décider quoi que ce soit : `secrets`, `docker`, `cicd.Run()` n'accèdent au filesystem que via `core.FileContext.Content`, déjà lu par `core.Scanner` — zéro I/O directe dans le chemin `Analyzer`. Mais ce n'est vrai que parce que ce code est écrit et revu avec discipline, pas parce que l'interface `Run(FileContext) []Finding` l'impose : rien, dans le langage, n'empêche du code compilé dans le même binaire d'importer `os`/`net`/`os/exec` directement et d'ignorer complètement le paramètre reçu. Un contrat Go est une convention de compilation, pas un sandbox d'exécution — cette distinction est le point de départ de toutes les décisions qui suivent.

Également vérifié : aucun `recover()` n'existe nulle part dans `core`/`cli`, et `core.Scanner.Scan()` appelle `a.Run(ctx)` de façon synchrone, sans timeout. Un panic ou une boucle infinie dans *n'importe quel* analyzer plante ou bloque tout `repoaudit` aujourd'hui — un fait déjà vrai avant cette phase, mais dont les conséquences changent de nature dès qu'un analyzer peut être du code tiers non audité plutôt que du code de ce repo.

## Décision : subprocess NDJSON, jamais de plugin Go natif

Le package `plugin` de Go (`.so` chargés dynamiquement) est exclu **définitivement**, pas juste pour cette v1 : aucun bénéfice de sécurité (même mémoire, mêmes privilèges que le process hôte), pas de support Windows, couplage strict de version de compilateur Go entre hôte et plugin, déconseillé par l'équipe Go elle-même. Aucun des trois axes (sécurité, portabilité, stabilité) ne penche en sa faveur.

Le transport retenu est un process séparé, communication NDJSON sur stdin/stdout, un process lancé **une fois par scan** (pas une fois par fichier — coût de spawn inacceptable sur un gros repo, même leçon que le endpoint batch OSV en Phase 3). Protocole détaillé dans `docs/plugin-protocol.md`, qui fait autorité pour un auteur de plugin externe — ce document-ci explique le *pourquoi*, pas le *comment*.

WASM (WASI) offrirait un sandboxing plus fort (deny-by-default sur filesystem/réseau/syscalls) — noté comme évolution possible, pas fermée, pas construite maintenant : un subprocess avec un protocole à octets seuls (voir plus bas) suffit à éliminer les fuites de capacité qui comptent pour ce périmètre, et est accessible à n'importe quel langage sans toolchain de compilation spécifique.

## Décision : octets seuls, jamais un chemin résolvable par le plugin

Le message `file` transmet le contenu en base64, jamais un chemin que le plugin résoudrait lui-même par sa propre I/O. C'est la clause qui rend l'isolation process réelle plutôt que théorique : si le protocole donnait un chemin et laissait le plugin faire son propre `open()`, on recréerait le problème du plugin natif par-dessus une couche RPC, pour rien. Documenté explicitement dans `docs/plugin-protocol.md` — pas une convention tacite qu'un auteur tiers pourrait rater en lisant vite.

Portée volontairement minimale pour cette v1 : lecture d'octets uniquement, aucun accès réseau, aucun accès filesystem étendu. Aucun des exemples de plugins cités par le vision.md (Terraform, manifests Kubernetes, analyse statique Python, règles enterprise) n'a besoin de plus qu'un accès lecture-octets par fichier. Une future extension (accès réseau pour enrichir un finding, par exemple) serait une négociation explicite du protocole, jamais une capacité ambiante par défaut.

## Décision : schéma de wire versionné, séparé de `core.Finding`

`protocol_version` négocié au handshake, distinct du struct Go interne : un plugin externe recompile indépendamment de RepoAudit, donc rien ne garantit qu'il tourne avec la même version de `core.Finding` qu'un analyzer interne (qui, lui, se recompile avec le reste du repo à chaque changement). Champs inconnus ignorés, pas rejetés — le schéma est fait pour évoluer sans casser les plugins déjà écrits.

Namespace des `id` de finding forcé côté hôte (préfixe `<plugin_name>.` ajouté automatiquement si absent) — pas une confiance aveugle dans la discipline d'un auteur tiers, un filet de sécurité anti-collision.

## Décision : un plugin qui échoue dégrade, ne fait jamais échouer le scan

Trois modes de défaillance, traités de façon identique : erreur fatale explicite du plugin, timeout (**5 secondes par fichier**, pas un budget global type git-history), ou crash du process. Les trois convergent structurellement vers la même fonction d'abandon côté hôte — un crash fait échouer la lecture sur stdout exactement comme un timeout, donc il n'y a pas de branche de code séparée à maintenir pour "le process est mort" versus "le process ne répond pas".

Pas de redémarrage après un abandon : un plugin qui crashe ou timeout sur un fichier a probablement un vrai problème qui se reproduira, retenter ajoute de la latence sans bénéfice attendu. Cohérent avec le pattern déjà établi (budget de temps du git-history analyzer, dégradation réseau du Dependency Scanner) : ne jamais laisser un contrôle défaillant faire tomber tout le scan.

**Conséquence explicite sur "zéro-config, ça marche toujours"** : cette garantie reste vraie pour le scan de base (secrets/docker/cicd, jamais affecté par un plugin tiers cassé), mais devient conditionnelle *pour les findings du plugin lui-même* dès qu'un utilisateur installe du code tiers de mauvaise qualité. RepoAudit ne peut garantir la fiabilité que de son propre code, pas de celui qu'un utilisateur choisit d'ajouter — c'est une limite assumée, pas un défaut à corriger.

## Ce que le sandboxing ne résout pas

Même parfaitement isolé (process séparé, octets seuls), un plugin peut *mentir* dans son résultat — sévérité gonflée, ou pire, un vrai problème masqué. Le sandboxing protège la machine de l'utilisateur, pas la véracité du résultat. C'est un risque inhérent à l'idée même d'accepter de la détection tierce, quel que soit le transport — aucune décision d'architecture ne l'élimine, seule la confiance dans l'auteur du plugin le fait.

## Hors scope de cette PR

Le symlink externe non géré dans `core.Scanner` (repéré pendant l'audit initial : un fichier du repo scanné qui serait un symlink vers `/etc/passwd` verrait son contenu réel lu et associé à un `Path` d'apparence innocente) — existant depuis la Phase 1, pas spécifique aux plugins, traité comme un sujet séparé sur demande explicite de l'utilisateur.

Découverte/installation de plugins (`repoaudit plugins list`/`install` du vision.md) — cette PR n'ajoute qu'un flag `--plugin <chemin-executable>`, répétable, pointant vers un binaire local déjà présent sur la machine. Un registre ou un mécanisme d'installation est une question séparée, non traitée ici.

## Conséquences

- Testé avec un plugin de référence écrit en **Python**, pas Go — validation honnête de la promesse "n'importe quel langage" plutôt qu'un test uniquement dans l'écosystème Go. Voir `docs/examples/reference-plugin.py` et `docs/testing.md`.
- Les 5 scénarios de défaillance (fatal au handshake, erreur non-fatale par fichier, timeout, crash, isolation entre plugins) validés avec des fixtures réelles, pas supposés.
- Stress-testé à 500 fichiers à travers un plugin réel (process Python) : 0.21s — coût du protocole négligeable, cohérent avec le principe déjà établi qu'un aller-retour sur pipe local coûte beaucoup moins qu'un aller-retour réseau.
