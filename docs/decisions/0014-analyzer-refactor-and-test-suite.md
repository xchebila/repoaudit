# ADR 0014 — `analyzers.BuiltinAnalyzers()` + première vraie suite de tests

## Statut

Accepté (2026-07-23).

## Contexte

Revue d'architecture externe (perspective staff/principal engineer) : rigueur de process excellente (ADR, validation empirique, corpus réel) mais couverture de tests automatisée quasi nulle — un vrai disqualifiant pour un outil de sécurité, indépendamment de la qualité du code. Décision : traiter ça avant toute nouvelle feature, dans l'ordre où c'est arrivé (refactor d'abord, tests dessus ensuite).

## Décision : `analyzers.BuiltinAnalyzers()` avant les tests, pas après

`cli/scan.go` et `analyzers/diffmode/diffmode.go` codaient chacun, indépendamment, `[]core.Analyzer{secrets.New(), docker.New(), cicd.New()}` — deux endroits à synchroniser à la main, sans aucun test pour attraper une divergence (un nouvel analyzer ajouté à un seul des deux serait passé inaperçu).

**Ne peut pas vivre dans `core` lui-même** : `secrets`/`docker`/`cicd` importent tous `core` — `core` les important en retour serait un cycle. Nouveau package `analyzers` (fichier racine `analyzers/builtin.go`), qui se situe au-dessus de `core` et des sous-packages d'analyzers — exactement là où `cli` et `diffmode` se trouvaient déjà, donc aucun des deux ne gagne de nouvelle dépendance.

Vérifié empiriquement avant de committer (pas seulement compilé) : scan et diff contre un vrai repo git jetable avec un secret, même détection qu'avant le refactor.

## Décision : fixtures de dépôts Git générées en Go, pas le corpus de repos réels

Le corpus de 20 repos publics (`docs/testing.md`) reste la référence pour valider vitesse/faux positifs sur du contenu réel — mais pour tester la *logique de code* (appariement de findings dans `diffmode`, dédup/mapping de sévérité dans `osv.go`), des fixtures Go générées à la volée (`t.TempDir()` + `go-git`, `git.PlainInit` + `Worktree.Commit`) sont strictement meilleures : déterministes, rapides, sans réseau, et surtout sans dépendre de `/tmp` qui a déjà disparu une fois pendant ce projet (Phase 2). Les 5 scénarios déjà documentés comme checklist manuelle dans `docs/testing.md` (secret ajouté/supprimé/préexistant/décalage de ligne/comptage) sont maintenant des `_test.go` réels dans `analyzers/diffmode/diffmode_test.go`.

## Décision : `osvBatchURL`/`osvVulnURL` passent de `const` à `var`

Seul changement de code de production motivé uniquement par la testabilité : ces deux URLs étaient des `const`, donc impossibles à rediriger vers un `httptest.Server` sans modifier `osv.go`. Passées en `var` (le reste du fichier reste `const`) — un seul point d'entrée pour les tests (`analyzers/dependencies/osv_test.go`), sans toucher à aucune signature de fonction publique. `TestQueryBatch_Chunking` est le test de non-régression direct du vrai bug trouvé contre l'API réelle (plafond de 1000 requêtes/batch, silencieusement dépassé sur prometheus) — la vraie API OSV.dev reste testée manuellement en pré-release (voir `docs/testing.md`), ce test-ci protège seulement la logique de chunking elle-même.

## Décision : golden files pour `output/json.go` et `output/html.go`

Protège les schémas de sortie versionnés (ADR 0009/0010) contre une régression silencieuse. `output/golden_test.go` définit une fixture `goldenFindings` unique, partagée par les deux formats, couvrant chaque champ notable (`commit_hash` rempli, `context` rempli, sévérités et catégories différentes). `output/testdata/report.{json,html}.golden` sont les références ; `go test ./output/... -update` les régénère après un changement de schéma volontaire.

**Un problème réel à résoudre, pas juste en théorie** : `WriteHTMLReport` embarque un timestamp (`time.Now()`), donc une comparaison d'octets stricte échouerait à chaque run. `normalizeHTMLTimestamp` remplace la date réelle par une valeur fixe avant comparaison (à l'écriture du golden comme à la lecture) — le seul champ dont la valeur doit légitimement changer à chaque exécution est neutralisé, pas ignoré aveuglément.

Validé comme un vrai test de non-régression, pas supposé : un titre cassé dans le template HTML fait échouer `TestWriteHTMLReport_Golden`, confirmé en cassant volontairement le template puis en le restaurant, avant de considérer le test fiable.

## Conséquences

- Couverture après ce travail : `secrets` 96.9%, `docker` 97.2%, `cicd` 95.2%, `diffmode` 78.6%, `dependencies` (logique pure d'`osv.go`) couverte, `output` (JSON + HTML) couvert par golden files — voir `docs/testing.md` pour le détail par fichier.
- Pas d'objectif de pourcentage global : un `cli` fin ou du code de câblage n'a pas besoin d'être poussé à un chiffre arbitraire pour que ce travail ait rempli son objectif.
- `githistory` et `plugin` restent sans test automatisé après cette PR — hors scope de cette itération, candidats naturels pour la suivante.
