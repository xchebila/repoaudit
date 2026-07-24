# ADR 0003 — Docker Analyzer : réutilisation, sévérités, une exception ciblée

## Statut

Accepté (2026-07-22).

## Contexte

Le Docker Analyzer (Phase 2) doit détecter : absence d'utilisateur non-root / `USER root`, tags `latest`, `ADD .` au lieu de `COPY`, secrets dans `ENV` (vision.md). Trois questions à trancher avant l'implémentation : faut-il dupliquer la détection de secrets pour le cas `ENV`, quelle sévérité pour chaque règle, et comment éviter les faux positifs sur les patterns Docker légitimes (multi-stage, images qui gèrent déjà leur propre utilisateur non-root).

## Décision

**"Secrets dans ENV" ne nécessite aucun code** : `core.Scanner` fait déjà tourner tous les analyzers enregistrés sur chaque fichier, un Dockerfile est un fichier texte comme un autre — `secrets.New()` détecte donc déjà `ENV AWS_KEY=AKIA...` avec `Category: "secrets"`. Vérifié empiriquement (`docs/decisions/0003`, voir tests). Le Docker Analyzer se limite aux 3 vérifications structurelles, `Category: "docker"`.

**Sévérités** : MEDIUM pour l'utilisateur root (aligné sur l'exemple du vision.md, "⚠️ MEDIUM - Docker runs as root"), LOW pour le tag `latest` et pour `ADD` au lieu de `COPY` — aucune de ces trois règles n'est une fuite immédiate, contrairement à un secret exposé (voir la checklist du skill `reposcan-finding`, point 1 : la sévérité doit refléter un risque réel, pas juste une mauvaise pratique).

**Garde-fous anti-FP**, validés sur un corpus réel (prometheus) et pas seulement en théorie :
- Tags : les noms de stage (`FROM x AS builder`, puis `FROM builder`) sont collectés avant l'évaluation, pour ne pas confondre une référence à un stage précédent avec une image non taguée.
- `ADD` : exclu si la source est une URL (`http(s)://`) ou une archive reconnue (`.tar`, `.tar.gz`, `.tgz`, `.zip`...) — ce sont les deux usages qu'`ADD` seul sait faire.
- `USER` : seul le dernier stage compte (un `USER root` suivi d'un `USER 1000` dans le même stage n'est pas flaggé — seule la dernière instruction avant EOF détermine l'utilisateur runtime réel).
- **Exception ajoutée après coup** : les images `distroless` documentent leur non-root via le tag (`:nonroot-*`), pas via une instruction `USER` visible dans le Dockerfile. Confirmé sur le `Dockerfile.distroless` réel de prometheus (commentaire explicite dans le fichier : "Base image sets USER to 65532:65532 (nonroot user)", aucune instruction `USER`). Le tag contenant `nonroot` (insensible à la casse) suffit à ne pas déclencher `docker.no_nonroot_user`.

**Limite assumée, non corrigée** : une image de base non-distroless qui fixe déjà un utilisateur non-root en interne (ex. `node:20-alpine` → `USER node`) sans que ça apparaisse dans son tag reste un faux positif possible — inspecter l'image de base elle-même est hors scope (pas d'analyse statique profonde, vision.md).

## Conséquences

- Aucune duplication de règles entre `secrets` et `docker` — le principe "réutiliser, pas redéfinir" déjà appliqué au git-history analyzer (ADR 0002) s'étend ici.
- Stress-testé à 2000 Dockerfiles générés (pire cas : les 3 règles se déclenchent sur chacun) : 0.6s. Pas de risque identifié de dépassement du budget `<5s` du MVP pour un monorepo microservices réaliste — contrairement au git-history analyzer, il n'y a pas d'axe "taille d'historique" qui puisse exploser ici, seulement le nombre de Dockerfiles (toujours petit et humain-écrit, même dans un gros monorepo).
- Le score reste une seule valeur combinée (pas de breakdown par catégorie) — conforme au roadmap, ce découpage est explicitement prévu en Phase 5, pas avant.
