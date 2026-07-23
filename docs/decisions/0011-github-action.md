# ADR 0011 — GitHub Action : packaging composite, `go install` direct, checkout full-history

## Statut

Accepté (2026-07-23).

## Contexte

Premier item de `docs/roadmap-long-term.md` (post-v1.0) : un GitHub Action officiel qui wrap le binaire `repoaudit` existant — aucune nouvelle feature CLI, du packaging pur. Le scope initial supposait "installe le binaire déjà existant", mais ce repo n'a aucune infra de release (pas de `.github/`, pas de release GitHub, pas de goreleaser) — cette hypothèse ne tenait pas et a dû être tranchée avant de coder, comme demandé.

## Décision : `go install` plutôt qu'un pipeline de binaires précompilés

Deux options en présence : monter un vrai pipeline goreleaser (binaires multi-plateformes précompilés, téléchargement rapide) ou `go install github.com/xchebila/repoaudit@<ref>` via `actions/setup-go` dans l'action composite elle-même. Choix : `go install`. Monter goreleaser est une vraie tâche d'infra à part entière, pas du packaging — au-delà du scope "aucune nouvelle feature" de cette phase. `go install` fonctionne dès aujourd'hui, sans rien inventer côté distribution ; le coût (15-30s de compilation par run) est acceptable pour un MVP. Le pipeline de release reste une amélioration future si le besoin de vitesse se confirme à l'usage.

**Conséquence découverte en cours de route** : le module Go s'appelait `repoaudit` (pas `github.com/xchebila/repoaudit`), donc non résolvable par `go install` depuis GitHub. Renommé dans `go.mod` et dans les 15 fichiers importateurs — mécanique mais nécessaire, vérifié par `go build ./...`, `go vet ./...` et `go test ./...` avant de continuer.

`GOPROXY=direct` sur l'étape d'installation (pas globalement) : contourne le cache du module proxy, qui peut retarder la résolution d'un commit tout juste poussé de quelques secondes à quelques minutes. Nécessaire pour que le propre workflow de dogfooding de ce repo (qui installe la branche en cours, pas un tag publié) fonctionne de façon fiable dès le push, et sans incidence sur un consommateur externe qui installe un tag déjà propagé.

## Décision : détection PR vs push via `github.event_name`, refs via les SHA de l'event payload

`github.event_name == 'pull_request'` (ou `pull_request_target`) bascule sur `repoaudit diff`, tout le reste sur `repoaudit scan`. Les refs de diff viennent de `github.event.pull_request.base.sha` / `.head.sha` — des SHA de commit réels issus du payload de l'event, jamais de `github.base_ref`/`github.head_ref` (des noms de branche, donc des refs mouvantes, sujettes à un force-push pendant l'exécution du job).

## Décision : l'action fait son propre checkout, en full history

Composite action qui inclut son propre `actions/checkout@v4` avec `fetch-depth: 0`, plutôt que de supposer que le workflow appelant l'a déjà fait avec la bonne profondeur. Sans ça, un clone shallow par défaut (`fetch-depth: 1`) n'a pas le commit de base d'une PR en local, et `repoaudit diff` (qui lit directement les objets git, jamais un checkout de travail — voir README) échouerait à le résoudre. Coût : plus lent sur un très gros repo — acceptable pour ce MVP, à revisiter seulement si ça devient une vraie plainte.

## Décision : validé par un vrai run CI, pas seulement en local

Comme chaque décision de packaging/infra de ce projet, un YAML "qui compile" ne prouve rien. `.github/workflows/repoaudit-self-check.yml` fait tourner l'action (`uses: ./`) sur les propres PR et push de ce repo — c'est aussi la seule façon réaliste de tester un vrai payload d'event GitHub, un vrai runner, et une vraie résolution `go install` depuis GitHub, plutôt que de le simuler localement.

**Ce test a immédiatement trouvé un vrai bug**, invisible en local : sur un event `pull_request`, `github.sha` pointe vers le commit de merge éphémère que GitHub construit pour le run (`refs/pull/N/merge`) — un commit qui n'existe sur aucune branche du dépôt distant, donc jamais résolvable par `go install`. Le fallback (invocation locale, `github.action_ref` vide) doit distinguer l'event : `github.event.pull_request.head.sha` sur une PR (le vrai commit poussé), `github.sha` sinon (correct sur un `push`, où c'est bien le commit réel). Exactement le genre d'erreur que la discipline "valider en CI réelle, pas en local" de ce projet existe pour attraper.

**Un deuxième bug, trouvé par le run suivant** : une fois le bon SHA résolu, `go install` échouait quand même — `sum.golang.org` renvoyait une 500 en tentant de vérifier le pseudo-version d'un commit tout juste poussé (le même type de problème de fraîcheur que `GOPROXY=direct` réglait déjà côté proxy, mais côté base de checksums, un mécanisme distinct). `GOSUMDB=off` sur cette même étape : sûr ici, puisque l'étape installe repoaudit depuis son propre dépôt connu, pas une dépendance tierce — aucune chaîne d'approvisionnement à vérifier. Sans rapport avec la vérification des dépendances auditées par `--deps` (qui passe par l'API OSV, pas par `cmd/go`).

## Conséquences

- `fail-on-new` contrôle si l'action échoue (`continue-on-error` conditionnel), pas si repoaudit tourne — repoaudit tourne toujours, le choix ne porte que sur l'échec du build.
- En mode `scan` (push), le rapport JSON est toujours uploadé en artifact, même si `fail-on-new: true` fait échouer le job (`if: !cancelled()`) — un job rouge doit rester inspectable.
- `--deps` n'a pas d'équivalent en mode `diff` (cf. limites déjà documentées dans le README) — l'input `deps` de l'action est donc ignoré sur un event `pull_request`.
- Ajout d'un Makefile minimal (`build`, `test`, `check`, `clean`) reproduisant la checklist manuelle déjà utilisée à chaque PR de ce projet (`go build ./... && go vet ./... && gofmt -l . && go test ./...`) — aucune nouvelle vérification, juste la même commande nommée une fois pour toutes.
