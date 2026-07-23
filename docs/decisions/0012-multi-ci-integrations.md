# ADR 0012 — GitLab CI / Jenkins : snippets documentés, pas de nouvel artefact publié

## Statut

Accepté (2026-07-23).

## Contexte

Deuxième item de `docs/roadmap-long-term.md`, à ne pas commencer avant le GitHub Action (ADR 0011) — le pattern `go install` + `diff`/`scan --format json` est maintenant validé en conditions CI réelles. Deux vraies questions d'architecture à trancher avant de coder, comme pour la Phase GitHub Action.

## Décision : un snippet documenté, pas un artefact publié, pour les deux plateformes

**GitLab** : un vrai "CI/CD Component" publié dans le catalog GitLab n'apparaît que si le projet qui le publie est hébergé *sur* gitlab.com, avec ses propres tags sémver et un pipeline de release. Le canonique de RepoAudit est sur GitHub — publier un Component exigerait de créer et maintenir indéfiniment un miroir hébergé sur gitlab.com, uniquement pour ce besoin.

**Jenkins** : aucun équivalent natif à une Action/un Component — l'option la plus proche est une Shared Library (du Groovy, chargée par `@Library`), un vrai code à maintenir dans un projet 100% Go jusqu'ici.

Choix pour les deux : un snippet documenté dans `docs/ci-integrations.md` (`.gitlab-ci.yml` et `Jenkinsfile`), à copier-coller — zéro nouvelle infra à maintenir, cohérent avec "packaging pur" de la phase précédente. À revisiter seulement si l'usage réel le demande.

## Décision : gérer la subtilité des SHA sur chaque plateforme, pas juste copier le pattern GitHub

**GitLab** a un piège structurellement proche du bug ADR 0011 (`github.sha` = commit de merge éphémère sur une PR), mais inversé selon le type de pipeline : en pipeline "merged results" (fréquent), `CI_COMMIT_SHA` est un commit de merge synthétique — mais `CI_MERGE_REQUEST_SOURCE_BRANCH_SHA` y est renseigné avec le vrai SHA source. En pipeline "merge request" classique, c'est l'inverse : cette variable est vide, et c'est `CI_COMMIT_SHA` qui est le vrai SHA source. Le snippet utilise donc `${CI_MERGE_REQUEST_SOURCE_BRANCH_SHA:-$CI_COMMIT_SHA}` — la variable la plus spécifique en priorité, repli sur l'autre. Vérifié par recherche (documentation GitLab), pas par essai-erreur — aucune instance GitLab disponible ici pour un vrai test end-to-end (voir Conséquences).

**Jenkins** n'a pas ce piège de résolvabilité (voir `docs/ci-integrations.md` pour le détail) : `repoaudit diff` lit directement l'objet git déjà présent localement après le checkout Jenkins, il n'y a rien à résoudre à distance comme pour le `go install` de l'Action GitHub. Le vrai point d'attention est différent et plus simple : `CHANGE_TARGET` est un nom de branche, pas un SHA (même piège que `github.base_ref`, volontairement évité aussi dans l'Action GitHub) — le snippet calcule le SHA de base réel via `git merge-base` plutôt que d'utiliser le nom de branche directement dans `repoaudit diff`.

**Correction en cours de rédaction** : la première version de ce document affirmait que Jenkins Multibranch checkoutait toujours le head brut de la PR, jamais un commit de merge synthétique — vérifié par recherche que c'est faux : la stratégie de checkout (PR head vs Merge Commit) est configurable par le plugin Branch Source utilisé. Corrigé pour dire que `repoaudit diff` fonctionne dans les deux cas (aucune résolution distante requise), mais que l'utilisateur doit savoir laquelle est configurée sur son job, puisque ça détermine ce que le diff compare réellement.

## Conséquences

- **Écart de validation assumé, pas caché** : contrairement à `action.yml` (prouvé par de vrais runs GitHub Actions, ADR 0011), aucun des deux snippets n'a tourné sur une vraie instance GitLab ou Jenkins — ce projet n'en a pas à disposition. Vérifiés par recherche documentaire et relecture, pas par exécution réelle. Écart déclaré explicitement dans `docs/ci-integrations.md`, pas présenté comme équivalent à la garantie de l'Action GitHub.
- Les deux snippets reprennent les mêmes choix que l'Action GitHub : `GOPROXY=direct` + `GOSUMDB=off` scopés à la seule commande d'installation (même justification qu'ADR 0011 — aucune chaîne d'approvisionnement tierce à vérifier pour installer son propre outil), rapport toujours généré même en cas d'échec (`artifacts: when: always` / `archiveArtifacts allowEmptyArchive`).
- Ni l'un ni l'autre n'a d'équivalent `--deps`/`--plugin` sur la branche "diff" du snippet, pour la même raison que `repoaudit diff` lui-même n'en a pas (voir README).
