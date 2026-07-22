# Benchmarks

Table append-only : une entrée par phase/PR, jamais réécrite. Un changement qui modifie une mesure précédente ajoute une nouvelle ligne datée, il ne corrige pas l'ancienne — l'historique des régressions/améliorations est aussi utile que le chiffre courant. Voir `docs/testing.md` pour le corpus et la méthodologie, `docs/decisions/` pour le raisonnement derrière chaque choix.

| Date | Phase/PR | Repo | Taille | Mode | Temps | Note |
|---|---|---|---|---|---|---|
| 2026-07-02 | Phase 1 (MVP secrets) | axios | shallow, 1 commit | scan working-tree | 0.29s | 1 finding (clé test) |
| 2026-07-02 | Phase 1 (MVP secrets) | caddy | shallow, 1 commit | scan working-tree | 0.29s | 3 findings (clés test) |
| 2026-07-02 | Phase 1 (MVP secrets) | chalk | shallow, 1 commit | scan working-tree | 0.04s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | cobra | shallow, 1 commit | scan working-tree | 0.07s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | express | shallow, 1 commit | scan working-tree | 0.09s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | fastapi | shallow, 1 commit, ~3000 fichiers | scan working-tree | 1.48s | propre — le plus lent du corpus, docs multilingues ; a motivé le passage au matching pleine-fichier + prefilter littéral (4.9s → 1.16s user) |
| 2026-07-02 | Phase 1 (MVP secrets) | fd | shallow, 1 commit | scan working-tree | 0.06s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | flask | shallow, 1 commit | scan working-tree | 0.14s | 1 finding (.env test) |
| 2026-07-02 | Phase 1 (MVP secrets) | fzf | shallow, 1 commit | scan working-tree | 0.14s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | gin | shallow, 1 commit | scan working-tree | 0.08s | 1 finding (clé test) |
| 2026-07-02 | Phase 1 (MVP secrets) | gitignore | shallow, 1 commit | scan working-tree | 0.07s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | glow | shallow, 1 commit | scan working-tree | 0.04s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | hugo | shallow, 1 commit | scan working-tree | 1.00s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | lazygit | shallow, 1 commit | scan working-tree | 0.36s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | lodash | shallow, 1 commit | scan working-tree | 0.15s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | ohmyzsh | shallow, 1 commit | scan working-tree | 0.39s | propre (1 FP doc trouvé puis corrigé — regex clé privée exigeait juste le header, pas un bloc complet) |
| 2026-07-02 | Phase 1 (MVP secrets) | prometheus | shallow, 1 commit | scan working-tree | 0.88s | 5 findings (4 clés test + .env test) |
| 2026-07-02 | Phase 1 (MVP secrets) | requests | shallow, 1 commit | scan working-tree | 0.12s | 4 findings (clés test) |
| 2026-07-02 | Phase 1 (MVP secrets) | ripgrep | shallow, 1 commit | scan working-tree | 0.21s | propre |
| 2026-07-02 | Phase 1 (MVP secrets) | svelte | shallow, 1 commit | scan working-tree | 1.46s | propre |
| 2026-07-03 | Phase 2 (git-history, calibrage) | cobra | complet, 1.1k commits | `--full-history`, depth=50 | 0.35s | a motivé le choix budget de temps plutôt que profondeur fixe |
| 2026-07-03 | Phase 2 (git-history, calibrage) | gin | complet, 2k commits | `--full-history`, depth=50 | 0.39s | idem |
| 2026-07-03 | Phase 2 (git-history, calibrage) | prometheus | complet, 18k commits, 1.6k fichiers | `--full-history`, depth=50 | 4.4s | même profondeur, 12x plus lent que cobra — `object.DiffTree` coûte selon la taille de l'arbre, pas le nombre de commits ; raison directe du choix "budget de temps" (`docs/decisions/0002`) |
| 2026-07-22 | Phase 2 (git-history, mode par défaut) | cobra | complet, 1.1k commits | scan par défaut (budget 1.5s) | 1.60s | 100/100, aucun finding — arrêté à 276 commits sur budget |
| 2026-07-22 | Phase 2 (git-history, mode par défaut) | gin | complet, 2k commits | scan par défaut (budget 1.5s) | 1.59s | 40/100 — arrêté à 209 commits sur budget |
| 2026-07-22 | Phase 2 (git-history, mode par défaut) | prometheus | complet, 18k commits | scan par défaut (budget 1.5s) | 2.47s (variance observée : 2.47–3.21s selon charge) | 0/100 — arrêté à ~30 commits sur budget (bien plus profond sur cobra/gin à budget égal, cf. variance de coût par commit) |
| 2026-07-22 | Phase 2 (git-history, `--full-history`) | cobra | complet, 1.1k commits | `--full-history` | 3.90s | propre |
| 2026-07-22 | Phase 2 (git-history, `--full-history`) | gin | complet, 2k commits | `--full-history` | 14.2s | 8 findings (clés de test, chemins distincts) |
| 2026-07-22 | Phase 2 (git-history, `--full-history`) | prometheus | complet, 18k commits | `--full-history`, après exclusion `vendor/` | 18m15s (818s user + 221s system) | 11 findings (clés test + .env test) ; gain de vitesse de l'exclusion vendor **non confirmé** (comparable au run précédent) — voir addendum `docs/decisions/0002` |
| 2026-07-22 | Phase 2 (Docker analyzer) | monorepo synthétique, 500 services | ~500 Dockerfiles multi-stage réalistes, propres | scan working-tree | 0.21s | 100/100 |
| 2026-07-22 | Phase 2 (Docker analyzer) | monorepo synthétique, 2000 services | ~2000 Dockerfiles, pire cas (3 règles déclenchées sur chacun) | scan working-tree | 0.61s (0.62s avant l'exception distroless, 0.61s après — pas de différence mesurable) | 74/100, 6000 findings |
| 2026-07-22 | Phase 2 (Docker analyzer) | prometheus | complet, 2 Dockerfiles réels | scan working-tree | (négligeable, inclus dans les lignes ci-dessus) | 1 vrai positif (`FROM ...:latest`), 1 FP trouvé et corrigé (`Dockerfile.distroless`, non-root via convention de tag, pas via `USER`) |
