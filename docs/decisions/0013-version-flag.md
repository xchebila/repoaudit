# ADR 0013 — `--version` : ldflags injectés, cohérents sur les trois chemins de build

## Statut

Accepté (2026-07-23).

## Contexte

Prérequis découvert en préparant la distribution Homebrew (`docs/roadmap-long-term.md`) : `brew test` a besoin d'une commande simple à faire tourner sur le binaire fraîchement construit, et `--version` était le candidat naturel — mais il n'existait pas (`unknown flag: --version`, vérifié empiriquement avant d'écrire quoi que ce soit).

## Décision : `cobra.Command.Version`, pas une sous-commande maison

`cli.NewRootCmd` prend maintenant un paramètre `version string` et le pose sur `root.Version` — Cobra enregistre `--version` automatiquement dès que ce champ est non vide, avec un format de sortie standard (`reposcan version <X>`). Pas de sous-commande `version` ni de flag custom à maintenir.

## Décision : injection par ldflags, pas `runtime/debug.ReadBuildInfo()`

Le projet a maintenant trois chemins de build distincts qui doivent tous rapporter une version cohérente : `go build` local (Makefile), `go install` (Action GitHub, ADR 0011), et bientôt `go build` depuis un tarball extrait sans `.git` (formula Homebrew). `runtime/debug.ReadBuildInfo()` aurait fonctionné pour `go install module@version` (Go embarque la version du module automatiquement) mais pas pour un `go build` local sans contexte VCS ni résolution de module versionnée — exactement le cas du build Homebrew depuis un tarball extrait. Les ldflags (`-X main.version=...`), injectés explicitement à chaque chemin de build, marchent identiquement partout et ne dépendent d'aucun mécanisme automatique différent d'un chemin à l'autre :

- **Makefile** (`make build`) : `git describe --tags --always --dirty`, donc un vrai tag en local, un hash court sinon.
- **Action GitHub** (`action.yml`) : `-ldflags "-X main.version=$ref"`, où `$ref` est déjà le SHA ou le tag résolu par la logique existante (ADR 0011) — aucune nouvelle donnée à calculer.
- **Formula Homebrew** (à venir) : `-ldflags "-X main.version=#{version}"`, la version déclarée par la formula elle-même.

`var version = "dev"` dans `main.go` reste la valeur par défaut pour un `go build`/`go run` sans ldflags — un dev qui compile localement sans passer par `make` voit "dev", pas une fausse version.

## Conséquences

- Vérifié empiriquement, pas seulement lu dans la doc Cobra : `go build -o reposcan . && ./reposcan --version` → `reposcan version dev` ; avec `-ldflags "-X main.version=v1.0.0"` → `reposcan version v1.0.0` ; `go install -ldflags "..." .` confirmé aussi (syntaxe valide, testée en local avec un `GOBIN` de scratch).
- Les snippets GitLab/Jenkins (`docs/ci-integrations.md`) n'injectent pas ces ldflags — laissés tels quels pour l'instant (déjà documentés comme non validés en conditions réelles, ADR 0012) ; `--version` y rapporterait "dev". À corriger si un besoin réel apparaît pour ces deux plateformes, même logique que pour le reste de cette roadmap.
