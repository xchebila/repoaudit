# ADR 0004 — Dependency Scanner : opt-in via `--deps`, jamais par défaut

## Statut

Accepté (2026-07-22). Décision prise avant l'implémentation (Phase 3, séquencé après le CI/CD Analyzer) pour ne pas avoir à la re-débattre une fois le code écrit.

## Contexte

Le Dependency Scanner (vision.md Phase 3) doit interroger l'API OSV.dev pour détecter des dépendances vulnérables. C'est la première feature de RepoAudit qui a besoin du réseau — tout le reste (secrets, git-history, docker, CI/CD à venir) est purement local.

Le git-history analyzer a déjà un précédent de "dégradation propre" (budget de temps, `Truncated` explicite) plutôt qu'un comportement tout-ou-rien. La question : le même principe de dégradation s'applique-t-il à un appel réseau, au point de le justifier comme comportement par défaut ?

## Décision

**Non — le Dependency Scanner est opt-in via `--deps`, jamais actif par défaut.** Le scan par défaut reste 100% local et déterministe, comme aujourd'hui.

Un budget de temps dégradé (git-history) et un appel réseau ne sont pas la même catégorie de risque :
- Le budget de temps dégrade un calcul **local et déterministe** — deux exécutions du même scan sur le même repo donnent le même résultat, juste éventuellement moins de commits couverts.
- Un appel réseau vers OSV.dev peut échouer pour des raisons hors du contrôle de l'utilisateur (proxy d'entreprise qui bloque l'accès sortant, latence variable, timeout), et le résultat peut **varier d'une exécution à l'autre sur le même repo** — ce qui casse la promesse implicite d'un scan reproductible.
- Un warning stderr récurrent sur un check que l'utilisateur n'a jamais demandé (ex. environnement CI sans accès réseau sortant) est du bruit imposé, pas du signal — contraire à "Signal > bruit".

La mécanique technique reste celle envisagée dès le départ, seulement déplacée derrière le flag :
- Endpoint batch OSV (`/v1/querybatch`) — une requête pour tout un `go.sum`/`requirements.txt`, pas une par dépendance.
- Timeout réseau court, dégradation propre (avertissement, pas d'échec dur) si le réseau est indisponible ou trop lent.

**Découvrabilité** : le scan par défaut affiche une ligne discrète en fin de rapport (`ℹ️ Run with --deps to also check dependencies against known vulnerabilities (requires network)`), pour que la feature reste trouvable sans être imposée.

## Conséquences

- Le scan par défaut garde sa promesse "sans setup, quelques secondes, toujours pareil" intacte — aucune régression de fiabilité en ajoutant le Dependency Scanner.
- Un utilisateur CI qui veut la vérification de dépendances doit l'activer explicitement (`repoaudit scan . --deps`) — c'est un coût de découvrabilité assumé en échange du déterminisme par défaut.
- Précédent posé pour toute future feature réseau (ex. GitHub Advisory DB, futures intégrations SaaS) : opt-in par défaut, jamais silencieusement actif.
