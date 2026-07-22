# ADR 0002 — Budget de temps plutôt que profondeur en nombre de commits

## Statut

Accepté (2026-07-03).

## Contexte

Le Git History Analyzer (Phase 2) doit rejouer le diff de chaque commit contre son premier parent pour retrouver des secrets committés puis supprimés. Sur un gros repo, marcher tout l'historique casse le critère "<5s" du MVP — il fallait une limite par défaut, avec `--full-history` pour lever la limite.

L'hypothèse de départ était une profondeur fixe en nombre de commits (ex. 500 ou 1000). Benchmark sur trois clones complets (cobra 1.1k commits, gin 2k, prometheus 18k, tous avec `object.DiffTree` + lecture des blobs modifiés) :

| Repo | Fichiers trackés | Temps pour 50 commits |
|---|---|---|
| cobra | quelques dizaines | 348ms |
| gin | ~100 | 394ms |
| prometheus | 1652 | **4.4s** |

Un même nombre de commits coûte jusqu'à 12x plus cher sur prometheus que sur cobra — `object.DiffTree` (basé sur merkletrie) doit ouvrir davantage d'objets tree pour localiser les mêmes changements dans un arbre plus large. Un plafond en nombre de commits fixe aurait donc été sûr sur les petits repos et dangereusement lent sur les gros — l'inverse de ce qu'on veut d'un garde-fou.

Profilage plus fin : même à taille de repo égale, le coût par commit varie fortement d'une exécution à l'autre (6 à 48 commits traités dans une fenêtre de temps quasi identique sur prometheus), probablement lié à la profondeur des chaînes de delta dans le packfile. Cette variance est elle-même imprévisible par un compteur de commits.

## Décision

Le scan d'historique par défaut est borné par un **budget de temps** (`DefaultBudget = 1.5s`, voir `analyzers/githistory/githistory.go`), pas par un nombre de commits. Le budget est vérifié entre deux commits (pas d'interruption en cours de traitement) ; un plafond dur secondaire (`hardCommitCeiling = 20000`) protège contre le cas pathologique inverse (énormément de commits individuellement très bon marché).

`--full-history` lève à la fois le budget de temps et le plafond dur, et ajoute un balayage des commits dangling (`repo.CommitObjects()`, tous les objets commit de l'object store, atteignables ou non depuis une ref — pas besoin de parser le reflog).

Le premier commit marché (HEAD) ne fait scanner que son côté "avant" (ce qu'il a potentiellement supprimé) : son côté "après" est par définition l'état actuel du working tree, déjà couvert par `core.Scanner`. Sans cette règle, tout secret encore présent aujourd'hui était compté deux fois (une fois par catégorie `secrets`, une fois par `git-history`), gonflant à tort la pénalité de score pour un seul incident réel.

## Conséquences

- Le budget s'adapte automatiquement à la taille et à la forme du repo, contrairement à un nombre de commits fixe — y compris sous charge système (dégrade en nombre de commits traités, jamais en dépassement de temps incontrôlé).
- La profondeur réellement couverte n'est pas déterministe (peut varier d'une exécution à l'autre sur le même repo). Le CLI compense en étant explicite : `result.Truncated` déclenche un avertissement visible indiquant que l'historique n'a pas été couvert en entier, avec `--full-history` comme échappatoire.
- Mesuré sur cobra/gin/prometheus (clones complets) : scan combiné (working tree + historique) systématiquement sous 2.5s, marge confortable sous les 5s.
- Cette variance de fond (system load, delta-chain depth) n'est pas totalement maîtrisable sans changer d'approche (ex. accès plumbing bas niveau) — accepté comme compromis MVP, à revisiter si le budget de 1.5s se révèle trop généreux ou trop serré en usage réel.

## Addendum — validation `--full-history` et exclusion `vendor/`

En validant `--full-history` sur les mêmes trois repos (au-delà du budget par défaut, donc sans garde-fou de temps), deux découvertes ont mené à des correctifs avant merge :

1. **`prometheus --full-history` a mis ~15 minutes**, pas quelques minutes comme anticipé — un commit de mise à jour de `vendor/` peut toucher plusieurs milliers de fichiers d'un coup, et `--full-history` n'a par design aucun budget pour les sauter. Ajout d'un indicateur de progression (`OnProgress`, "scanné N/total commits") pour que ce ne soit pas indiscernable d'un hang — un vrai gap UX, indépendant du fait que la lenteur elle-même soit un compromis assumé de ce mode.
2. **Deux faux positifs confirmés dans du code vendoré**, absents du corpus Phase 1 (qui ne scannait que le working tree courant) : une clé AWS d'exemple officielle (`AKIAIOSFODNN7EXAMPLE`, convention documentée par AWS — tout exemple qu'ils publient se termine par `EXAMPLE`) dans `vendor/github.com/aws/aws-sdk-go`, et une fixture de test Google (`"project_id": "project_id"`, `client_id: "123"`) dans `vendor/google.golang.org/api`.

Plutôt que de patcher au cas par cas, le scan d'historique applique désormais la même exclusion `vendor/`/`node_modules` que `core.Scanner` applique déjà au working tree en Phase 1 (`core.IsVendoredPath`, vérifié avant même la décompression des blobs). Ce n'est pas un contournement du principe de l'ADR 0001 (pas de dégradation de sévérité sur un pattern de chemin) : c'est une exclusion complète, déjà actée pour le working tree — l'étendre à l'historique est une question de cohérence, pas un nouvel arbitrage.

Correctness confirmée : re-testé sur prometheus `--full-history`, plus aucun match sous `vendor/` dans les findings. **Gain de vitesse non confirmé en revanche** : le run complet a pris 18m15s (818s user + 221s system), pas mesurablement plus rapide que l'estimation de l'exécution précédente (~15min, interrompue avant d'avoir un chiffre final exact faute de baseline propre). Hypothèse la plus probable : le coût dominant est le parcours d'arbre de `object.DiffTree` (merkletrie) pour localiser les changements dans un commit de vendor bump, pas la décompression des blobs eux-mêmes que l'exclusion évite — sauter la lecture de contenu n'élimine donc qu'une fraction du coût réel. Sans conséquence sur la promesse `<5s` : c'est uniquement `--full-history` (déjà explicitement "sans budget") qui est concerné, le mode par défaut n'est pas affecté par cette limite.
