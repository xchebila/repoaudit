# Testing — corpus, critères de sortie, où trouver quoi

Ce fichier centralise comment RepoAudit est validé contre des repos réels, pour ne pas avoir à reconstruire cette connaissance à chaque phase (le corpus complet a déjà été perdu une fois, en Phase 2, quand `/tmp` a été vidé par une interruption de session).

Les décisions de design elles-mêmes (pourquoi un budget de temps plutôt qu'une profondeur fixe, pourquoi ne pas dégrader la sévérité sur un pattern de chemin...) vivent dans `docs/decisions/` — ce fichier n'y touche pas, il n'y renvoie que.

## Corpus de 20 repos publics (Phase 1)

Utilisé pour valider le critère de sortie du MVP. Clones shallow (`--depth 1`) suffisants pour tester le scan working-tree — pour tester le git-history analyzer (Phase 2+), il faut des clones complets (voir plus bas).

```
spf13/cobra
gin-gonic/gin
junegunn/fzf
sharkdp/fd
BurntSushi/ripgrep
jesseduffield/lazygit
charmbracelet/glow
prometheus/prometheus
gohugoio/hugo
caddyserver/caddy
pallets/flask
psf/requests
tiangolo/fastapi
expressjs/express
sveltejs/svelte
axios/axios
lodash/lodash
chalk/chalk
ohmyzsh/ohmyzsh
github/gitignore
```

Pour le reconstituer :

```bash
mkdir -p /tmp/corpus && cd /tmp/corpus
while read -r repo; do
  name=$(basename "$repo")
  git clone --depth 1 --quiet "https://github.com/$repo.git" "$name"
done <<'EOF'
spf13/cobra
gin-gonic/gin
junegunn/fzf
sharkdp/fd
BurntSushi/ripgrep
jesseduffield/lazygit
charmbracelet/glow
prometheus/prometheus
gohugoio/hugo
caddyserver/caddy
pallets/flask
psf/requests
tiangolo/fastapi
expressjs/express
sveltejs/svelte
axios/axios
lodash/lodash
chalk/chalk
ohmyzsh/ohmyzsh
github/gitignore
EOF
```

Repos avec des findings secrets légitimes (clés de test dans `testdata/`/`fixtures/`, `.env` de test) : axios, caddy, flask, gin, prometheus, requests. Les autres scannent propres. Utile pour repérer immédiatement une régression : si l'un des repos "propres" se met à remonter un finding, c'est un faux positif à investiguer avant de merger, pas un nouveau vrai positif à célébrer.

`caddy`, `gin` et `prometheus` ont des `.gitignore` avec des patterns de négation (`!fichier`) — utiles pour valider concrètement le warning `.gitignore` plutôt que de le laisser théorique.

## Clones complets (Phase 2+, git-history)

Le git-history analyzer a besoin de vrai historique, pas d'un clone shallow. Trois tailles représentatives, déjà utilisées pour calibrer le budget de temps (voir `docs/decisions/0002-git-history-depth.md`) :

| Repo | Commits | Fichiers trackés |
|---|---|---|
| cobra | ~1.1k | ~66 |
| gin | ~2k | ~130 |
| prometheus | ~18k | ~1.6k |

```bash
mkdir -p /tmp/corpus-full && cd /tmp/corpus-full
git clone --quiet https://github.com/spf13/cobra.git
git clone --quiet https://github.com/gin-gonic/gin.git
git clone --quiet https://github.com/prometheus/prometheus.git
```

`prometheus` est le cas qui a révélé la plupart des limites réelles jusqu'ici (vendor bump massif dans l'historique, faux positifs dans du code vendoré, Dockerfile réel avec un tag `latest` et un `Dockerfile.distroless`) — c'est le premier repo à re-tester dès qu'un changement touche `githistory` ou `docker`.

## Dockerfiles et workflows réels dans le corpus Phase 1

Le corpus de 20 repos sert aussi à valider `docker` et `cicd` contre du contenu réel, pas seulement des fixtures synthétiques — 9 des 20 ont au moins un vrai workflow GitHub Actions (axios, caddy, chalk, cobra, flask, gin, ohmyzsh, prometheus, requests ; 57 fichiers `.yml` au total). C'est ce qui a révélé que `gin/.github/workflows/codeql.yml` et `requests/.github/workflows/codeql-analysis.yml` contiennent tous les deux `@main`/`@master` dans un contexte qui n'est pas une référence d'action (`branches: [main]`, un commentaire) — la justification empirique du parsing YAML structurel plutôt que regex, voir `docs/decisions/0005-cicd-analyzer-scope.md`.

## Critères de sortie mesurables (déjà validés)

- **Vitesse < 5s** (critère de sortie du MVP, vision.md) : validé sur les 20 repos du corpus Phase 1 (max observé : ~1.5s, fastapi/svelte) et sur les clones complets en mode par défaut (max observé : ~3s, prometheus — budget git-history de 1.5s + scan working-tree + overhead process). `--full-history` n'est **pas** soumis à ce critère : c'est un mode explicitement "sans budget", jusqu'à 18 minutes observées sur prometheus (18k commits) — voir `docs/decisions/0002-git-history-depth.md`.
- **Zéro faux positif majeur** : validé sur les 20 repos Phase 1 après correctifs (extension `.pem`/`.key` confondue avec certificat, regex de clé privée matchant un placeholder de doc, clé AWS d'exemple officielle et fixture Google dans du code vendoré). Ce qui reste est une classe connue et documentée (clés de test dans `testdata/`/`fixtures/`) — voir `docs/decisions/0001-test-fixture-context.md` pour pourquoi ce n'est pas supprimé, seulement annoté.
- **Budget de temps : per-analyzer, pas global.** `DefaultBudget` (1.5s) est interne à `githistory.Scan()` — le scanner working-tree (`core.Scanner`) n'a aucun budget propre, chaque analyzer enregistré (`secrets`, `docker`) tourne sans limite de temps individuelle. Le garde-fou aujourd'hui est indirect : chaque analyzer reste, par construction, un parsing léger (regex + prefilter littéral, pas de parsing profond), validé empiriquement à chaque nouvel ajout plutôt que supposé. Si un futur analyzer s'avérait intrinsèquement plus coûteux par fichier, il faudrait alors introduire un budget explicite au niveau du `Scanner` — pas fait aujourd'hui car aucun analyzer ne l'a justifié jusqu'ici.

## Où sont les chiffres

`docs/benchmarks.md` — table append-only, un run par phase/PR. Ce fichier-ci dit *quoi* tester et *pourquoi* ; benchmarks.md dit *ce qui a été mesuré, quand*.
