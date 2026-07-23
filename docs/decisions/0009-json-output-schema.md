# ADR 0009 — JSON output : schéma de wire versionné, séparé de `core.Finding`

## Statut

Accepté (2026-07-23).

## Contexte

`--format json` (Phase 5, vision.md) sérialise les findings et le score pour un consommateur machine (script CI, autre outil). Vision.md propose une commande séparée `repoaudit report --format json/html` — non retenu ici : il n'existe aucune persistance de scan passé à "reporter" plus tard, et chaque phase précédente a étendu les commandes existantes plutôt qu'introduire un nouvel arbre de commandes. `--format json` est un flag sur `scan` (pas sur `diff`, hors scope de cette PR).

## Décision

`output/json.go` définit son propre struct JSON (`jsonFinding`, `jsonReport`) plutôt que d'ajouter des tags `json:"..."` directement sur `core.Finding`, avec un `schema_version` explicite. Même raisonnement que le protocole plugin (ADR 0008) : un consommateur externe qui parse `--format json` ne recompile pas avec ce repo — rien ne garantit qu'il suive un changement de nom de champ interne fait pour la lisibilité du code Go. Coût marginal (un struct + une fonction de conversion), cohérent avec un principe déjà établi deux fois dans ce projet plutôt qu'une nouvelle réflexion.

Chaque champ de `Finding` est toujours présent dans le JSON, même vide (`context: ""`, `commit_hash: ""`) — pas de `omitempty`. Un consommateur ne doit pas avoir à deviner si un champ absent signifie "non applicable" ou "supprimé cette fois".

## Conséquences

- Validé avec de vraies fixtures couvrant tous les champs simultanément : un finding avec `commit_hash` réel (git-history) et `context` réel (chemin test/fixture) sérialise correctement chaque champ.
- Validé aussi via un plugin externe (Phase 4) : l'`id` namespacé (`reference-example.example_marker_found`) et le `category` par défaut (nom du plugin) traversent le même chemin de sérialisation sans cas particulier.
- Les diagnostics (warnings `.gitignore`, dépendances, historique git, plugins) restent sur stderr, jamais mêlés au JSON de stdout — vérifié en séparant explicitement les deux flux, pas supposé.
