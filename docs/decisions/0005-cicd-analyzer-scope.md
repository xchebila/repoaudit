# ADR 0005 — CI/CD Analyzer : YAML structurel plutôt que regex, absence de Dependabot hors du modèle par-fichier

## Statut

Accepté (2026-07-22).

## Contexte

Le CI/CD Analyzer (Phase 3) doit détecter : permissions de workflow trop larges (`write-all`), actions non pinées (`@main`), secrets exposés dans les logs de build, absence de config Dependabot. Deux questions d'architecture distinctes des analyzers précédents : comment lire le YAML (regex ligne par ligne, comme Docker, ou parsing structurel), et comment exprimer une vérification "ce fichier n'existe nulle part dans le repo" dans un modèle d'analyzer conçu pour tourner par fichier.

## Décision

**Parsing YAML structurel (`yaml.v3`), pas regex ligne par ligne.** Contrairement à Docker (syntaxe simple, une instruction par ligne), une clé `permissions`/`uses`/`run` en YAML peut apparaître dans un commentaire (`# uses: x@main` en exemple de doc) ou dans une valeur qui n'a rien à voir (`branches: [main]` est un déclencheur de workflow, pas une référence d'action). Confirmé concrètement sur le corpus réel : `gin/.github/workflows/codeql.yml` et `requests/.github/workflows/codeql-analysis.yml` contiennent tous les deux `@main`/`@master`, mais dans un commentaire (exemple de syntaxe du template CodeQL officiel de GitHub) et dans `branches: [master]` — une regex sur le texte brut aurait produit un faux positif sur les deux ; le parsing structurel les ignore correctement puisqu'aucun n'est une vraie clé `uses:`.

L'implémentation ne modélise pas le schéma complet workflow/job/step : `walk()` cherche les clés `permissions`/`uses`/`run` n'importe où dans l'arbre. Aucune des trois règles n'a besoin de savoir si le match est au niveau du workflow ou d'un job précis — `permissions: write-all` est aussi risqué à un endroit qu'à l'autre.

**"Secrets dans ENV" ne nécessite aucun code**, même principe qu'avec Docker (`docs/decisions/0003`) : un workflow est un fichier texte comme un autre, `secrets.New()` le scanne déjà.

**"Absence de Dependabot" sort du modèle `core.Analyzer` par fichier.** `Run(file core.FileContext)` reçoit un fichier à la fois — il n'y a aucun point d'accroche pour dire "après avoir vu tous les fichiers, ce fichier précis n'est jamais apparu". `CheckDependabot(repoRoot string)` est donc une fonction de vérification au niveau du repo, appelée directement depuis `cli/scan.go`, exactement comme `githistory.Scan()` (même raison : un besoin qui ne rentre pas dans l'interface par-fichier).

## Sévérités

- `permissions: write-all` → HIGH (misconfiguration directement exploitable : un token compromis peut écrire n'importe où).
- Action non pinée (`@main`/`@master`) → MEDIUM (risque supply-chain réel, mais nécessite que l'upstream soit compromis pour s'exercer — pas aussi immédiat qu'un write-all).
- Secret dans les logs → MEDIUM, pas CRITICAL : GitHub masque les valeurs de secrets connues dans les logs (mitigation réelle bien qu'imparfaite), contrairement à un secret réellement hardcodé en clair.
- Absence de Dependabot → LOW (absence d'une bonne pratique, pas une mauvaise configuration active).

## Faux positifs vérifiés, pas supposés

- Un faux positif suspecté au départ (`echo "token set: ${{ secrets.TOKEN != '' }}"`, une vérification booléenne) s'est avéré **ne pas se produire** : la regex `echoSecretPattern` exige que `}}` suive immédiatement le nom du secret, donc `!= ''` avant `}}` empêche le match. Confirmé avec une fixture synthétique plutôt que supposé — le commentaire de code initial était inexact et a été corrigé.
- Limite réelle, non corrigée : une indirection via variable shell sur plusieurs lignes (`TOKEN="${{ secrets.TOKEN }}"` puis `echo $TOKEN` sur une ligne différente) n'est pas détectée — la vérification est ligne par ligne, sans suivre le flux de variables shell. Accepté comme limite MVP, cohérent avec le refus d'analyse statique profonde (Non-Goals, vision.md).
- Action non pinée d'un repo de sa propre organisation (`@main` sur une action interne de confiance) reste un faux positif possible, non corrigé : impossible de distinguer de manière fiable "même organisation" depuis un seul fichier de workflow.

## Conséquences

- Nouvelle dépendance actée explicitement dans vision.md (`gopkg.in/yaml.v3`), pas ajoutée silencieusement.
- Stress-testé à 500 workflows générés (3 règles déclenchées sur chacun) : 0.14s — même profil de coût que Docker, aucun risque de dépassement du budget `<5s`.
- Validé sur le corpus réel des 9 repos ayant des workflows (axios, caddy, chalk, cobra, flask, gin, ohmyzsh, prometheus, requests) : aucune régression sur les findings existants, `No Dependabot` correctement absent sur les 6 repos qui en ont un, correctement présent sur les 3 qui n'en ont pas.
