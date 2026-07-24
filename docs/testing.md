# Testing — corpus, critères de sortie, où trouver quoi

Ce fichier centralise comment RepoScan est validé contre des repos réels, pour ne pas avoir à reconstruire cette connaissance à chaque phase (le corpus complet a déjà été perdu une fois, en Phase 2, quand `/tmp` a été vidé par une interruption de session).

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

## Règles internes (secrets/docker/cicd) : tests table-driven, depuis l'ADR 0014

Le corpus réel valide la vitesse et l'absence de faux positifs à grande échelle ; il ne protège pas contre une régression sur une règle précise (une regex resserrée qui casse une exclusion, une logique multi-stage Dockerfile qui se dérègle). `analyzers/secrets/secrets_test.go`, `analyzers/docker/docker_test.go` et `analyzers/cicd/cicd_test.go` couvrent maintenant chaque règle et chaque exclusion de faux positif documentée en commentaire (suffixe `EXAMPLE` d'AWS, corps PEM tronqué, `FROM builder` multi-stage, tag `nonroot` distroless, `ADD` d'URL/archive, `permissions` en map scopée vs `write-all`, vérification booléenne d'un secret vs `echo` direct) — couverture 95-97% sur les trois packages. `analyzers/cicd/cicd_test.go` couvre aussi `CheckDependabot`, la fonction niveau-repo appelée directement par `cli/scan.go`.

## Dependency Scanner : test contre l'API OSV.dev réelle, pas de mock

`analyzers/dependencies` fait de vrais appels réseau (`--deps`) — validé contre la vraie API `api.osv.dev`, jamais mockée. Deux dépendances volontairement anciennes/vulnérables servent de fixtures de référence pour ce chemin de code : `golang.org/x/text@v0.3.0` (Go) et `urllib3==1.24.1` (Python), toutes deux avec plusieurs CVE connues et couvrant les trois cas de mapping de sévérité (`database_specific.severity` direct, heuristique CVSS, fallback Medium — voir `docs/decisions/0006-dependency-scanner-scope.md`).

`gin` (57 dépendances) et `prometheus` (1075 dépendances sur 5 `go.sum`) du corpus servent aussi de test d'échelle réel pour ce chemin — c'est prometheus qui a révélé le plafond de batch OSV non documenté (1000 requêtes max) : sans le découpage en chunks, le check de dépendances échouait entièrement en silence sur ce repo, avec un message trompeur ("réseau indisponible").

Depuis l'ADR 0014, ce bug précis a aussi un test de non-régression automatisé : `TestQueryBatch_Chunking` (`analyzers/dependencies/osv_test.go`) rejoue le découpage en chunks contre un `httptest.Server` fake qui rejette toute requête de plus de 1000 dépendances — `osvBatchURL`/`osvVulnURL` sont des `var` (pas des `const`) précisément pour permettre ça, seul changement de code de production motivé par la testabilité. Le reste de la logique pure d'`osv.go` (dédup d'alias, mapping de sévérité, heuristique CVSS) est couvert dans le même fichier. La vraie API OSV.dev reste testée manuellement en pré-release, sans mock — c'est ce test-réseau réel qui a trouvé le plafond de batch en premier lieu ; le fake HTTP protège seulement la logique de chunking déjà découverte, il ne la remplace pas comme méthode de découverte de nouveaux bugs.

## Security Diff Mode : fixtures Git générées en Go, plus le corpus pour la perf

Les 5 scénarios ci-dessous étaient une checklist manuelle jusqu'à l'ADR 0014 — ce sont maintenant de vrais tests automatisés dans `analyzers/diffmode/diffmode_test.go`, contre des dépôts Git générés à la volée (`t.TempDir()` + `go-git`, pas de clone, pas de dépendance à `/tmp` qui a déjà disparu une fois) :
1. Un secret ajouté sur la branche → `NEW` (`TestDiff_NewSecret`).
2. Un secret supprimé sur la branche → `FIXED` (`TestDiff_FixedSecret`).
3. Un problème préexistant sur les deux branches, non touché par la branche → absent du diff (`TestDiff_PreexistingIssueIsNotReported`).
4. Un secret inchangé mais dont le numéro de ligne se décale → absent du diff, pas de faux `NEW`+`FIXED` (`TestDiff_LineShiftDoesNotFalselyReport`).
5. Deux findings de même clé `(File, ID, Category)`, un supprimé → exactement 1 `FIXED` (`TestDiff_CountAwarePairing`).

`prometheus` (clone complet, deux tags de version distants) reste la référence pour le test de perf sur un vrai gros repo — voir `docs/benchmarks.md`. Le corpus réel et les fixtures générées ne se remplacent pas : l'un valide la performance/les faux positifs sur du contenu réel, l'autre valide la logique pure d'appariement de façon déterministe et rapide.

## Plugin System : plugin de référence en Python, pas seulement du Go

`docs/examples/reference-plugin.py` — écrit en Python, délibérément, pour valider honnêtement la promesse "le protocole n'a rien de spécifique à Go" plutôt que de la laisser comme une affirmation non vérifiée dans `docs/plugin-protocol.md`. Accepte un flag `--misbehave=timeout|crash|fatal|error` pour rejouer chacun des scénarios de défaillance ci-dessous à la demande — pas destiné aux vrais auteurs de plugins, uniquement à la suite de test.

Six scénarios de référence à reproduire si on touche `analyzers/plugin` :
1. Fonctionnement normal → le finding du plugin apparaît dans le rapport, avec l'`id` correctement préfixé par `plugin_name`.
2. Erreur fatale au handshake (`--misbehave=fatal`) → plugin ignoré au chargement, scan continue normalement.
3. Erreur non-fatale sur un fichier (`--misbehave=error`) → warning par fichier concerné, plugin reste actif pour les fichiers suivants.
4. Timeout (`--misbehave=timeout`, le script dort 30s) → abandon après exactement 5s, pas de nouvelle tentative sur les fichiers suivants.
5. Crash (`--misbehave=crash`, `sys.exit(1)`) → détecté immédiatement (EOF sur stdout), pas d'attente du timeout.
6. Deux plugins en même temps, un qui crashe et un qui fonctionne → isolation confirmée, le crash de l'un n'affecte pas les findings de l'autre.

`--plugin` prend un chemin d'exécutable, pas une commande avec arguments (un vrai plugin est autonome, il n'a pas besoin de flags CLI) — pour tester chaque mode il faut donc un petit wrapper par mode plutôt que de passer `--misbehave=X` directement :

```bash
for mode in fatal crash timeout error; do
  cat > /tmp/reference-$mode.sh <<EOF
#!/bin/sh
exec python3 "$(pwd)/docs/examples/reference-plugin.py" --misbehave=$mode
EOF
  chmod +x /tmp/reference-$mode.sh
done
./reposcan scan . --plugin /tmp/reference-crash.sh
```

## Critères de sortie mesurables (déjà validés)

- **Vitesse < 5s** (critère de sortie du MVP, vision.md) : validé sur les 20 repos du corpus Phase 1 (max observé : ~1.5s, fastapi/svelte) et sur les clones complets en mode par défaut (max observé : ~3s, prometheus — budget git-history de 1.5s + scan working-tree + overhead process). `--full-history` n'est **pas** soumis à ce critère : c'est un mode explicitement "sans budget", jusqu'à 18 minutes observées sur prometheus (18k commits) — voir `docs/decisions/0002-git-history-depth.md`.
- **Zéro faux positif majeur** : validé sur les 20 repos Phase 1 après correctifs (extension `.pem`/`.key` confondue avec certificat, regex de clé privée matchant un placeholder de doc, clé AWS d'exemple officielle et fixture Google dans du code vendoré). Ce qui reste est une classe connue et documentée (clés de test dans `testdata/`/`fixtures/`) — voir `docs/decisions/0001-test-fixture-context.md` pour pourquoi ce n'est pas supprimé, seulement annoté.
- **Budget de temps : per-analyzer, pas global, mais plus indirect depuis l'ADR 0016.** `DefaultBudget` (1.5s) reste interne à `githistory.Scan()`. Le scanner working-tree (`core.Scanner`) et `diffmode.scanTree()` passent maintenant chaque appel d'analyzer par `core.RunAnalyzer`, qui abandonne l'attente (log + skip, jamais de hang) si un analyzer ne répond pas dans `AnalyzerTimeout` (5s, aligné sur le budget du protocole de plugin). Avant l'ADR 0016, ce garde-fou était entièrement indirect ("chaque analyzer reste un parsing léger, validé empiriquement à chaque ajout") — identifié dans une revue d'architecture externe comme le risque le plus sérieux et le plus silencieux du projet : rien n'empêchait structurellement un futur analyzer de bloquer tout le scan. Vérifié avec un analyzer factice "bloqué" 10s dans `core/timeout_test.go` : le scan complet revient en ~60ms avec les findings de l'analyzer normal, pas les 10s du bloqué.

## JSON output : validité + isolation stdout/stderr

`--format json` se valide sur trois points, tous vérifiés avec de vraies commandes plutôt que par lecture de code :
1. Le JSON produit est syntaxiquement valide (`python3 -m json.tool` en pipe).
2. `findings: []` sur un repo propre, jamais `null` — un consommateur ne doit pas avoir à gérer les deux cas.
3. Les diagnostics (`.gitignore`, dépendances, git-history, plugins) restent sur stderr, jamais mêlés au JSON de stdout — vérifié en séparant explicitement les deux flux (`2>/dev/null` vs `2>&1 1>/dev/null`), pas supposé.

Fixture de référence pour couvrir tous les champs simultanément : un mini-repo avec une clé de test ajoutée puis supprimée dans `testdata/` — produit un finding avec `commit_hash` réel (git-history) *et* `context` réel (chemin test/fixture) en même temps, pour confirmer que chaque champ traverse la sérialisation correctement.

Depuis l'ADR 0014, cette même fixture multi-champs est aussi un golden file automatisé : `output/golden_test.go` définit `goldenFindings` (un finding par champ notable : `commit_hash` rempli, `context` rempli, sévérités différentes, catégories différentes), et `output/json_test.go`/`output/html_test.go` comparent la sortie réelle à `output/testdata/report.{json,html}.golden`. Validité syntaxique du JSON toujours vérifiée directement (`encoding/json.Unmarshal`), pas seulement par comparaison au golden. `go test ./output/... -update` régénère les golden files après un changement de schéma volontaire. Testé pour de vrai, pas supposé : casser temporairement un titre dans le template HTML fait échouer `TestWriteHTMLReport_Golden` comme attendu, confirmé avant d'écrire cette note.

## Dashboard HTML : premier test Go automatisé du projet, plus une vérification structurelle du rendu

`core/scoring_test.go` — premier fichier `_test.go` de ce projet, écrit spécifiquement pour un invariant que la vérification CLI habituelle ne couvre pas bien : la garantie qu'aucun finding n'est compté deux fois ni oublié quand `ComputeCategoryBreakdown` partitionne par catégorie. Deux tests :
1. `TestComputeCategoryBreakdown_PartitionsWithoutDuplicationOrLoss` — utilise `ComputeCategoryScore` comme oracle : chaque score de catégorie dans le breakdown doit être identique à celui obtenu en filtrant manuellement les findings de cette catégorie.
2. `TestComputeCategoryBreakdown_TotalIsNotAnAggregateOfCategoryScores` — un CRITICAL dans une catégorie, des LOW ailleurs ; le score total doit diverger nettement de la moyenne naïve des catégories (35 contre 78 sur la fixture retenue), preuve que le total n'est jamais dérivé du breakdown.

Le HTML généré lui-même se valide en deux temps, aucun des deux par simple lecture de code :
- Bien-formé structurellement : tags équilibrés. Vérifié une première fois manuellement avec le parseur HTML de la stdlib Python (`html.parser`) ; depuis l'ADR 0014, `assertBalancedHTML` (`output/html_test.go`) fait la même vérification en Go à chaque run de `go test`, sans dépendance externe.
- Rendu visuel inspecté via un Artifact publié temporairement — cet environnement n'a pas d'outil de capture d'écran, donc la vérification pixel-perfect (alignement, espacement) reste à la charge de la review humaine ; l'automatisé couvre la structure et la logique couleur/statut, pas la mise en page. Le timestamp `generated <date>` du template est normalisé avant comparaison au golden file (`normalizeHTMLTimestamp`), sinon chaque run produirait un diff sur la seule valeur qui doit changer à chaque run.

## Où sont les chiffres

`docs/benchmarks.md` — table append-only, un run par phase/PR. Ce fichier-ci dit *quoi* tester et *pourquoi* ; benchmarks.md dit *ce qui a été mesuré, quand*.
