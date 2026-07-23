# ADR 0007 — Security Diff Mode : clé d'appariement, exit code, réutilisation

## Statut

Accepté (2026-07-22).

## Contexte

`repoaudit diff main feature-branch` (vision.md Phase 3) doit montrer ce qu'une branche/PR introduit ou corrige, pas un score statique du repo entier. Deux questions tranchées avec l'utilisateur avant l'implémentation : comment apparier un finding entre les deux refs pour décider NEW/FIXED/inchangé, et sur quoi baser le code de sortie.

## Décision : lire les refs via go-git, pas de `git checkout`

Réutilise l'infrastructure déjà écrite pour `githistory` (Phase 2) : lire l'arbre d'un commit via `commit.Tree()` puis `Tree.Files()`, construire des `core.FileContext` virtuels, faire tourner les analyzers existants (`secrets`, `docker`, `cicd`) dessus. Aucune écriture sur le disque, aucun risque d'écraser du travail en cours dans le répertoire de travail de l'utilisateur. `repo.ResolveRevision()` accepte nativement branches, tags, et hash de commit — pas de logique de résolution de ref à écrire.

Le Dependency Scanner (`--deps`) n'est délibérément pas inclus par défaut ici — même raisonnement que l'ADR 0004 (réseau = opt-in), pas un nouvel arbitrage.

## Décision : clé d'appariement `(File, ID, Category)`, sans `Line`

Comparer sur la ligne exacte ferait apparaître un faux `FIXED`+`NEW` pour un secret parfaitement inchangé si une ligne sans rapport est ajoutée plus haut dans le même fichier (décalage de tous les numéros de ligne suivants). Vérifié concrètement avec une fixture synthétique : un secret sur la ligne 5, des imports ajoutés faisant passer le secret à la ligne 7, contenu du secret inchangé → diff vide, comme attendu.

**Limite assumée, non corrigée** : un fichier renommé entre les deux refs sans modification de contenu apparaît comme `FIXED` (ancien chemin) + `NEW` (nouveau chemin) plutôt que comme inchangé — pas de détection de renommage, cohérent avec les Non-Goals du vision.md (pas d'analyse statique profonde).

**Comptage, pas ensemble** : si une clé `(File, ID, Category)` a 2 occurrences sur `refA` et 1 sur `refB`, exactement 1 est reportée `FIXED` (le surplus), pas 0 (ce qui perdrait l'information qu'une occurrence a bien disparu) ni 2 (ce qui compterait à tort l'occurrence encore présente comme résolue). Vérifié avec une fixture à deux clés AWS identiques dans le même fichier, une supprimée.

## Décision : exit code = tout `NEW`, sans seuil de sévérité

`diff` échoue (code 1) si au moins un finding `NEW` existe, quelle que soit sa sévérité — y compris un `NEW` en LOW (ex. tag `latest` introduit). Choix explicite de fail-closed strict plutôt que le seul choix possible : un flag `--fail-on=<severity>` (ex. n'échouer qu'à partir de Medium) est une extension plausible si le besoin apparaît en usage réel, mais pas ajouté maintenant — zéro config par défaut, cohérent avec la boussole UX du vision.md. Noté ici explicitement pour ne pas avoir à redébattre du compromis depuis zéro si la demande revient.

## Conséquences

- Testé sur repo synthétique : NEW détecté, FIXED détecté, problème préexistant non touché par la branche correctement absent du diff, décalage de ligne sans faux positif, comptage correct sur doublons.
- Testé sur repo réel (prometheus, deux tags de version distants) : 2.47s — pas de risque de perf identifié, `scanTree` a le même profil de coût que le scan working-tree déjà validé (regex légères, pas de parsing profond), juste exécuté deux fois (une par ref) plutôt qu'une.
- Pas de couverture git-history (secrets supprimés puis réintroduits entre les deux refs mais absents des deux arbres finaux) — Security Diff Mode compare deux états ponctuels, pas l'historique entre eux ; ce chemin reste la responsabilité de `githistory`/`--full-history`, pas un chevauchement à combler ici.
