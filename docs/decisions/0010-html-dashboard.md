# ADR 0010 — Dashboard HTML : breakdown par catégorie, autonome, statuts fixes

## Statut

Accepté (2026-07-23).

## Contexte

`--format html` (Phase 5, vision.md) rend les findings et le score comme un fichier HTML autonome. Cette PR concrétise aussi une promesse différée deux fois (ADR 0003, ADR 0005) : le breakdown par catégorie ("Secrets 10/10, Docker 6/10...") que le vision.md prévoit explicitement pour la Phase 5, pas avant.

## Décision : breakdown par catégorie sans toucher au moteur de scoring

`core.Score` avait déjà un champ `Category` jamais utilisé, et `ComputeCategoryScore(findings []Finding) Score` était déjà générique sur n'importe quel sous-ensemble de findings. `ComputeCategoryBreakdown` (nouveau, dans `core/scoring.go`) partitionne les findings par `Category` puis appelle `ComputeCategoryScore` sur chaque partition — zéro changement du moteur de scoring lui-même.

**Le score total n'est pas dérivé du breakdown.** `ComputeCategoryScore` est appelé une deuxième fois, indépendamment, sur la liste complète des findings — jamais comme une somme ou une moyenne des scores par catégorie. Une moyenne aurait dilué un CRITICAL d'une catégorie par des catégories propres ailleurs, exactement ce que le principe de scoring du vision.md interdit ("un critique doit dominer, pas s'additionner à égalité"). Vérifié par un test explicite (`TestComputeCategoryBreakdown_TotalIsNotAnAggregateOfCategoryScores`, `core/scoring_test.go`) : sur une fixture avec un seul CRITICAL dans une catégorie et des LOW ailleurs, le score total (35) diverge nettement de la moyenne naïve des catégories (78) — la preuve que les deux calculs sont bien indépendants, pas juste "ça compile".

## Décision : partitionnement vérifié par un test explicite, premier test Go du projet

Demande explicite avant merge : prouver qu'aucun finding n'est compté deux fois ni oublié dans le partitionnement par catégorie — pas seulement "ça compile et ça a l'air cohérent visuellement". `TestComputeCategoryBreakdown_PartitionsWithoutDuplicationOrLoss` utilise `ComputeCategoryScore` lui-même comme oracle : le score de chaque catégorie dans le breakdown doit être identique à celui obtenu en filtrant manuellement les findings de cette catégorie et en les scorant directement. Si le partitionnement dupliquait ou perdait un finding, les deux calculs divergeraient. C'est le premier fichier `_test.go` de ce projet — jusqu'ici toute validation passait par des scans manuels contre des fixtures réelles/synthétiques (efficace pour trouver de vrais bugs tout au long des phases précédentes), mais un invariant structurel sur une fonction pure est exactement le cas où un test automatisé est mieux adapté qu'une vérification CLI.

## Décision : fichier autonome, palette de statuts fixe, formes issues du skill dataviz

- **Zéro dépendance externe** (CSS inline, pas de CDN/police/JS) — cohérent avec le reste du projet, fonctionne hors-ligne, pas de nouvel appel réseau.
- **`html/template` de la stdlib**, pas de nouvelle dépendance de templating.
- Formes choisies par la table de décision du skill dataviz, pas au jugé : le score total est un **hero figure** ("le nombre que le dashboard porte en premier", ≥48px, chiffres proportionnels jamais tabulaires), le breakdown par catégorie est un **meter** par catégorie ("un ratio contre une limite" — exactement le cas d'un score /100).
- Couleurs de statut reprises telles quelles de la palette de référence du skill (`good #0ca30c`, `warning #fab219`, `serious #ec835a`, `critical #d03b3b`) — pas de nouvelle couleur inventée, donc pas de re-validation nécessaire (ce sont déjà les valeurs validées de référence).
- Chaque badge de sévérité associe icône + couleur + texte (jamais la couleur seule) — reprend exactement les deux icônes déjà utilisées par `output/cli.go` (❌ Critical/High, ⚠️ Medium/Low), pour une cohérence visuelle entre `--format cli` et `--format html`.
- Mode sombre via `prefers-color-scheme` uniquement (pas de toggle applicatif — c'est un fichier statique ouvert localement, pas une page servie avec un sélecteur de thème).

## Conséquences

- HTML généré vérifié structurellement bien formé (tags équilibrés) sur une fixture multi-catégories réelle (secrets + docker + cicd), pas seulement "ça compile".
- Rendu visuel inspecté via un Artifact temporaire (cet environnement n'a pas d'outil de capture d'écran) — limite assumée : la vérification pixel-perfect reste à la charge de la review humaine, la vérification automatisée couvre la structure et la logique de couleur/statut, pas la mise en page.
- `--format html` écrit sur stdout, comme `--format json` — cohérent, pas de nouveau flag `--output`.
