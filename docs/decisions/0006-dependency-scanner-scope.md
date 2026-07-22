# ADR 0006 — Dependency Scanner : mapping de sévérité, scope de parsing, dégradation réseau

## Statut

Accepté (2026-07-22). Complète l'ADR 0004 (déjà posée avant l'implémentation : opt-in via `--deps`), avec les décisions prises pendant l'implémentation elle-même.

## Contexte

Le Dependency Scanner interroge OSV.dev pour `go.sum` et `requirements.txt`. Trois questions ouvertes à l'implémentation : comment mapper la sévérité OSV (hétérogène selon la source) vers notre enum à 4 niveaux, quel scope de parsing pour les manifests, et comment structurer les appels réseau (batch + détails).

## Sévérité OSV : hétérogène, vérifié sur de vraies données avant de coder

`POST /v1/querybatch` ne renvoie que `{id, modified}` — un `GET /v1/vulns/{id}` par ID distinct est nécessaire pour le résumé et la sévérité. Vérifié sur deux vrais enregistrements avant d'écrire le code (pas supposé) :
- Une CVE GHSA : `database_specific.severity = "MODERATE"` (chaîne simple) **et** un vecteur CVSS complet.
- `GO-2023-1571` (base de vulnérabilités Go native) : **aucune sévérité du tout**.

Décision (validée avec l'utilisateur avant implémentation) :
1. `database_specific.severity` (CRITICAL/HIGH/MODERATE/LOW) si présent → mapping direct, pas de `Context`.
2. Sinon, vecteur CVSS présent → heuristique grossière sur les métriques d'impact (`C`/`I`/`A`) et le vecteur d'attaque (`AV`), **pas un vrai calcul CVSS** (implémenter la formule pondérée FIRST pour 3 versions de CVSS serait de la sur-ingénierie pour un MVP) → `Context` explicite : "severity estimated from a CVSS vector, not an official severity rating".
3. Sinon (aucune donnée) → Medium par défaut, `Context` explicite : "no severity information available... defaulted to Medium".

Cette hiérarchie réutilise le champ `Context` déjà en place pour le cas test/fixture (ADR 0001) — même principe : ne jamais faire passer une déduction approximative pour une donnée certaine sans le signaler, le score reste impacté pareil.

**Validé empiriquement, pas juste en théorie** : un test contre l'API réelle (`golang.org/x/text@v0.3.0`, `django==2.0.1`, `requests==2.6.0`) a fait apparaître les trois chemins simultanément — des CVE GHSA avec sévérité directe, des vulnérabilités Go natives (`GO-2020-0015`, etc.) tombant sur le fallback Medium, et des entrées PYSEC (Python natif) avec seulement un vecteur CVSS, donc passant par l'heuristique.

## Scope de parsing : versions pinnées uniquement

`go.sum` : dédoublonné sur `module@version` (une ligne hash + une ligne `/go.mod` par version, sinon double comptage). `requirements.txt` : seules les lignes `package==version` sont retenues — une dépendance non pinnée (`package>=1.0`) est **ignorée**, pas interrogée sans version. Interroger OSV sans version renverrait toutes les vulnérabilités jamais publiées pour ce paquet, y compris celles déjà corrigées dans la version réellement installée — exactement le genre de résultat ambigu que les Non-Goals du vision.md excluent.

Node est explicitement hors scope pour cette itération (vision.md : "Go, Python, puis Node en option").

## Découverte locale séparée de la vérification réseau

`Discover(repoRoot)` (marche le repo, parse les manifests) ne fait aucun appel réseau et tourne toujours, même sans `--deps` — c'est ce qui permet d'afficher "Found N dependencies" en mode par défaut sans jamais toucher le réseau. `CheckVulnerabilities(deps)` est la seule fonction qui appelle OSV.dev, uniquement sous `--deps`. Cette séparation est directement ce que demandait l'ADR 0004.

## Dégradation réseau : vérifiée, pas supposée

Testé concrètement en pointant temporairement le client vers un host invalide : le scan continue, affiche `⚠️  dependency check skipped: ...` sur stderr, et se termine normalement (code de sortie déterminé par les autres findings, pas un échec forcé). Conforme à l'ADR 0004.

Un cas plus fin : si la requête batch réussit mais qu'un fetch de détail individuel échoue (réseau flaky sur un seul appel parmi N), le Finding n'est **pas supprimé silencieusement** — il est produit en mode dégradé (sévérité Medium, `Context` explicite que les détails n'ont pas pu être récupérés). Une vulnérabilité confirmée par la requête batch ne doit pas disparaître à cause d'un échec secondaire.

## Bug réel trouvé en testant sur prometheus : limite de taille de batch non documentée

`prometheus` a 5 `go.sum` (module principal + sous-modules), 1075 dépendances au total. Premier test réel : la dégradation réseau s'est déclenchée (`⚠️  dependency check skipped: OSV.dev returned 400 Bad Request`) — silencieusement absorbée comme "réseau indisponible", alors que la vraie cause était différente et plus grave : **zéro vulnérabilité n'était remontée alors qu'il y en avait réellement** (confirmé en soumettant des sous-ensembles de la même liste directement à l'API : dès 1000 requêtes, des CVE réelles apparaissent).

Cause exacte, isolée par bissection contre l'API réelle (non documentée dans les pages OSV consultées) : `POST /v1/querybatch` rejette toute requête de plus de 1000 entrées avec `{"code":3,"message":"too many queries"}`. Corrigé en découpant les requêtes en chunks séquentiels de `maxBatchSize = 1000`. Sans ce découpage, tout repo avec plus de 1000 dépendances pinnées (facilement atteint par un monorepo Go multi-module) aurait vu son check de dépendances échouer intégralement, en silence, avec un message trompeur ("réseau indisponible" au lieu de "trop de dépendances pour une seule requête").

## Deuxième bug réel trouvé en préparant l'exemple du README : la même vulnérabilité comptée deux fois

En construisant un exemple de score pour le README (`urllib3==1.24.1`, volontairement ancien et vulnérable), deux findings affichaient un résumé identique mot pour mot : `GHSA-2xpw-w6gg-jr37` et `PYSEC-2026-1994`, tous les deux "urllib3 streaming API improperly handles highly compressed data". Vérifié contre l'API réelle : `GHSA-2xpw-w6gg-jr37.aliases` contient `["CVE-2025-66471", "PYSEC-2026-1994"]` — OSV indexe la même vulnérabilité sous plusieurs schémas d'ID (GHSA, PYSEC, CVE, les ID natifs `GO-*`), et une requête batch peut faire remonter plusieurs de ces ID pour le même paquet+version en même temps.

Corrigé avec `dedupeAliases` : pour chaque dépendance, les ID dont les `aliases` se recoupent avec d'autres ID retournés pour cette même dépendance sont fusionnés, en gardant un seul représentant (préférence pour un ID `GHSA-`, qui a généralement `database_specific.severity` renseigné — sinon choix arbitraire mais déterministe). Sans ce fix, `urllib3==1.24.1` remontait ~19-20 findings au lieu de 12 réels, gonflant à tort à la fois le nombre de findings et la pénalité de score pour des incidents comptés en double.

## Conséquences

- Appels aux détails de vulnérabilité parallélisés (8 workers concurrents) — go-idiomatique (vision.md : "Parallélisation : goroutines"), nécessaire dès qu'un repo a plusieurs dizaines de vulnérabilités distinctes à enrichir.
- Testé sur le corpus réel : gin (57 dépendances, ~3s), prometheus (1075 dépendances sur 5 `go.sum`, nécessite le découpage en chunks ci-dessus) — voir `docs/benchmarks.md` pour les chiffres exacts.
- Limite assumée, non corrigée : l'heuristique CVSS est volontairement approximative, jamais un vrai score. Si elle se révèle trop imprécise en usage réel, la corriger explicitement plutôt que la laisser dériver silencieusement vers un vrai calcul CVSS ad hoc.
