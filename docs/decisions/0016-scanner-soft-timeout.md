# ADR 0016 — `core.RunAnalyzer` : budget de temps souple, partagé entre scan et diff

## Statut

Accepté (2026-07-23).

## Contexte

Dernier point d'une revue d'architecture externe : `docs/testing.md` admettait déjà que le garde-fou contre un analyzer lent était "indirect... validé empiriquement à chaque nouvel ajout" — rien n'empêchait structurellement un futur analyzer (ou un plugin bugué dans son propre code, pas dans le protocole) de faire exploser le budget des 5s du MVP, ou pire, de faire hang le scan entier. Identifié comme le risque architectural le plus sérieux du projet, précisément parce qu'il est silencieux jusqu'au jour où un vrai repo utilisateur le révèle.

## Décision : un helper partagé, pas une garde dupliquée dans `core.Scanner` et `diffmode`

`core.RunAnalyzer(a Analyzer, file FileContext) []Finding` — même raisonnement que `analyzers.BuiltinAnalyzers()` (ADR 0014) : `core.Scanner.Scan()` et `analyzers/diffmode.scanTree()` bouclent chacun sur les mêmes analyzers pour le même fichier ; dupliquer la logique de timeout aux deux endroits aurait recréé exactement le problème que ce refactor venait de corriger. Vit dans `core` (nouveau fichier `core/timeout.go`) puisque c'est là que vivent déjà `Analyzer`, `Finding`, `FileContext` — `diffmode` importe déjà `core`, aucune nouvelle dépendance.

## Décision : "jamais de hang" ne veut pas dire "l'analyzer est tué"

Go n'a aucun moyen de forcer l'arrêt d'une goroutine en cours. Si `a.Run(file)` ne revient pas dans le budget (`AnalyzerTimeout`, 5s — aligné sur le budget déjà existant du protocole de plugin pour rester cohérent), la goroutine continue de tourner en arrière-plan, mais l'appelant n'attend plus dessus : un warning est loggé sur stderr et ce fichier est ignoré pour cet analyzer, le scan continue. C'est exactement le même compromis que le protocole de plugin (`analyzers/plugin/plugin.go`) fait pour un subprocess bloqué — sauf que là-bas, le process OS peut être réellement tué (`cmd.Process.Kill()`), alors qu'ici seule l'attente est abandonnée. Le warning va directement sur `os.Stderr`, sans passer par un mécanisme de warnings retourné (`Scanner.Warnings()`, la signature de `diffmode.Diff`) — même convention que `plugin.abandon()` déjà établie dans ce projet.

**Limitation assumée, spécifique à l'usage CLI actuel** : la goroutine abandonnée fuit — elle continue de tourner tant que `a.Run(file)` ne revient pas (jamais, dans le cas d'un vrai hang). Ce n'est pas un problème aujourd'hui parce que `repoaudit` est un process one-shot : la goroutine fuyante meurt avec le process quand `main()` retourne, à la fin du scan. Ce ne serait plus vrai si RepoAudit était un jour utilisé comme librairie Go dans un process long-vivant (un serveur qui appelle `core.Scanner.Scan()` en boucle, par exemple) — chaque timeout laisserait une goroutine fuir indéfiniment plutôt que de mourir avec le process, une vraie fuite de ressources cumulative. Pas le cas d'usage actuel (CLI uniquement, ni vision.md ni roadmap-long-term.md n'envisagent RepoAudit comme librairie embarquée) — donc pas à corriger maintenant, mais si cet usage apparaît un jour, ce compromis devra être revisité en premier (probablement via un `context.Context` annulable propagé dans `Analyzer.Run`, ce qui changerait la signature de l'interface — un changement bien plus lourd qu'aujourd'hui, à ne pas anticiper sans besoin réel).

## Décision : `AnalyzerTimeout` est une `var`, pas une `const`

Seul changement motivé par la testabilité (même pattern qu'ADR 0014 pour `osvBatchURL`/`osvVulnURL`) : un test qui attendrait réellement 5 secondes pour vérifier le chemin de timeout ralentirait la suite à chaque run, pour rien. Les tests réduisent temporairement `AnalyzerTimeout` (ex. 50ms) plutôt que d'attendre la vraie valeur.

## Conséquences

- Vérifié à trois niveaux, pas seulement "ça compile" : `RunAnalyzer` seul (timeout déclenché, findings nil), `RunAnalyzer` dans le budget (findings retournés normalement), et `core.Scanner.Scan()` complet avec un analyzer "bloqué" 10s à côté d'un analyzer normal — confirmé que `Scan()` revient en ~60ms avec le finding de l'analyzer normal, pas les 10s du bloqué.
- Testé empiriquement contre ce repo lui-même après le changement : même score (97/100), pas de régression de timing perceptible sur un scan normal.
- `githistory` n'utilise pas ce helper : il appelle `secrets.New().Run()` directement par composition (voir ADR 0002), pas via `core.Scanner`/`analyzerList`. Hors scope de cette ADR — même raisonnement de risque s'appliquerait si `githistory` gagnait un jour plusieurs analyzers au lieu d'un seul réutilisé tel quel.
