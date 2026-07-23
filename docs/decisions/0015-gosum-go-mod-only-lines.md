# ADR 0015 — `parseGoSum` ignore les lignes go.mod-only : elles ne sont jamais du code réellement construit

## Statut

Accepté (2026-07-23).

## Contexte

Issue #13 (dépendances transitives vulnérables : `x/text`, `yaml.v2`, `x/sys`) demandait de vérifier qu'un simple `go get -u` suffisait. Après `go get -u golang.org/x/text golang.org/x/sys` + `go mod tidy`, `repoaudit scan . --deps` continuait de signaler `golang.org/x/text@v0.3.6` et `gopkg.in/yaml.v2@v2.2.2` comme vulnérables — des versions qui ne sont **pas** celles réellement sélectionnées par la résolution de modules (`go list -m` confirme `v0.40.0` pour x/text ; `go mod why -m gopkg.in/yaml.v2` confirme que ce module n'est même pas atteignable depuis le graphe d'imports de ce projet).

## Décision

`go.sum` contient deux types de ligne par version de module : une ligne de contenu réel (`v1.2.3 h1:...`, le module effectivement téléchargé et compilé) et une ligne go.mod-seul (`v1.2.3/go.mod h1:...`, seulement le hash du fichier go.mod de cette version, lu pour résoudre le graphe de dépendances mais jamais téléchargé ni compilé). Une version peut n'exister qu'en ligne go.mod-seul indéfiniment — l'ancienne exigence go.mod d'une dépendance transitive, remplacée partout ailleurs par MVS (Minimal Version Selection) — sans jamais disparaître de `go.sum`, confirmé empiriquement : `go mod tidy` ne les retire pas, ce sont des lignes de vérification de graphe légitimes, pas des résidus.

L'ancienne version de `parseGoSum` traitait chaque version mentionnée n'importe où dans `go.sum` comme une dépendance en usage. Corrigé : une ligne dont le second champ se termine par `/go.mod` est ignorée entièrement — seule une ligne de contenu réel signifie qu'une version a été effectivement sélectionnée et compilée.

## Conséquences

- `x/text@v0.3.6` et `yaml.v2@v2.2.2` (ce dernier totalement fantôme, jamais atteignable depuis ce projet) ont disparu du scan — c'était deux faux positifs, pas des dépendances obsolètes à mettre à jour.
- Une fois ce bug corrigé, un vrai finding est apparu, auparavant noyé dans le bruit : `golang.org/x/net@v0.53.0` (version réellement utilisée), corrigé par un `go get -u` classique.
- **Résidu connu, non corrigé ici** : `golang.org/x/crypto@v0.54.0` reste signalé pour GO-2026-5932 ("openpgp package... unsafe by design"), un avis qui porte sur le sous-package `golang.org/x/crypto/openpgp` spécifiquement — vérifié via `go list -deps ./...` que ce sous-package n'est même pas dans le graphe d'imports de ce projet (seuls `hkdf`, `ssh`, `blake2b` etc. le sont). C'est une limite structurelle plus large : RepoAudit scanne au niveau du module, pas du sous-package/chemin d'import, et un avis OSV peut porter sur un seul sous-package d'un module par ailleurs sain. Pas un bug à corriger dans ce même effort — suivi séparément (issue dédiée), le scope de cette ADR est la distinction contenu réel vs go.mod-seul, pas le filtrage par chemin d'import.
- `TestParseGoSum_SkipsGoModOnlyLines` (`analyzers/dependencies/dependencies_test.go`) est le test de non-régression direct, avec les deux cas réels (x/text, yaml.v2) comme fixtures plutôt que des exemples inventés.
