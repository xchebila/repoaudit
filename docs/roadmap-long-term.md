# 🌍 Roadmap long terme — RepoAudit

Ce document couvre ce qui vient après le v1.0 (Phases 1-5, voir `vision.md`).

**Différence de nature avec vision.md** : les Phases 1-5 avaient chacune un scope technique clair, un critère de sortie mesurable, et un ordre imposé par les dépendances entre elles. Rien ici n'a ce niveau de certitude au départ — ce sont des directions, pas des engagements. Deux items (GitHub Action, CI multi-plateforme) étaient assez mûrs pour être cadrés comme de vraies phases, et sont maintenant faits ; les trois restants restent au niveau "direction + risque à trancher avant de commencer", volontairement, pour ne pas donner une fausse impression de planning détaillé sur des sujets encore ouverts.

---

## ✅ Fait — GitHub Action officiel

### Scope MVP

```yaml
- uses: repoaudit/action@v1
  with:
    fail-on-new: true    # utilise `repoaudit diff` si base-ref/head-ref détectés (PR), sinon `repoaudit scan`
```

- Action composite (pas de runtime custom) : installe le binaire `repoaudit` déjà existant, l'exécute, expose le code de sortie.
- Sur une pull request : utilise `repoaudit diff <base> <head>` (Phase 3, déjà livré) — le mode conçu précisément pour ce cas d'usage.
- Hors PR (push sur une branche) : bascule sur `repoaudit scan . --format json`, publié en artifact de build.
- Aucune nouvelle feature côté CLI — ce travail est du packaging pur autour de ce qui existe déjà (`diff`, `--format json`).

### Pourquoi ce n'est pas un audit de conception comme Phase 4

Contrairement au Plugin System, il n'y a pas de code tiers non fiable à isoler ici — l'action exécute le binaire officiel, publié par le mainteneur du projet, dans l'environnement CI de l'utilisateur qui l'a explicitement choisi. Pas de nouvelle surface de risque à trancher avant de coder.

### Critère de sortie — validé

- `fail-on-new: true` fait échouer le check GitHub exactement comme `repoaudit diff` en local (même code de sortie, même sémantique) — vérifié par un vrai run CI, pas seulement en local.
- Testé sur ce repo lui-même en conditions CI réelles via `.github/workflows/repoaudit-self-check.yml` (les trois chemins : PR/diff, push/scan, `workflow_dispatch`) — deux vrais bugs trouvés et corrigés en cours de route (SHA de merge éphémère, fraîcheur de sum.golang.org). Voir [docs/decisions/0011-github-action.md](decisions/0011-github-action.md).

---

## ✅ Fait — Intégration CI multi-plateforme (GitLab, Jenkins)

Snippets documentés (`docs/ci-integrations.md`), pas un artefact publié (pas de GitLab CI/CD Component, pas de Jenkins Shared Library) — décision et raisons dans [docs/decisions/0012-multi-ci-integrations.md](decisions/0012-multi-ci-integrations.md). Contrairement au GitHub Action, ni le snippet GitLab ni le snippet Jenkins n'ont tourné sur une vraie instance — écart de validation assumé et déclaré, pas cette même garantie de "testé en CI réelle".

---

## 🧭 Directions à auditer avant de coder

### Marketplace de plugins

**Statut** : nécessite un audit de conception avant tout code, comme Phase 4.

**Pourquoi** : "découverte/installation de plugins" a été explicitement mis hors scope lors de l'audit Phase 4 (ADR 0008) — c'est exactement cette question qui revient ici, pas une nouvelle idée indépendante.

**Risque central à trancher** : le protocole d'isolation (Phase 4) protège contre un plugin buggé ou malveillant *une fois lancé*, mais rien ne protège contre la découverte et l'installation d'un plugin déjà compromis. Publier un plugin sur un marketplace introduit une question de confiance (signature, review, sandboxing du marketplace lui-même) que le protocole d'exécution ne résout pas.

**Ne pas commencer** sans repasser par le même format que l'audit Phase 4 : questions posées explicitement, réponses vérifiées empiriquement quand possible, décision actée dans un ADR avant la première ligne de code.

---

### Extension VSCode

**Statut** : pas commencé.

**Différence structurelle avec le reste du projet** : stack technique différente (TypeScript, API VSCode) plutôt qu'une extension naturelle du code Go existant — pas juste une nouvelle feature, une nouvelle compétence à mobiliser.

**Question à trancher avant de commencer** : est-ce un wrapper léger autour du binaire CLI existant (comme le GitHub Action packaging), ou une vraie réimplémentation de logique côté extension ? La première option est beaucoup moins risquée et cohérente avec le principe "core minimal, ne pas dupliquer" déjà appliqué partout ailleurs dans le projet.

---

### SaaS optionnel

**Statut** : à requestionner avant même de commencer à cadrer quoi que ce soit.

**La vraie question, pas encore posée** : quel problème un SaaS résout-il que CLI + GitHub Action ne résolvent pas déjà ? Le vision.md le qualifie lui-même de "non obligatoire" — ce n'est pas un engagement, c'est une option qui n'a pas encore de justification claire.

**Tension avec la philosophie du projet** : un SaaS introduit un hébergement, potentiellement des comptes utilisateurs, une surface d'attaque et une charge opérationnelle sans rapport avec "CLI local, zéro-config, zéro dépendance" qui est au cœur de RepoAudit depuis le vision.md original. Avant tout cadrage technique, ce point mérite une vraie réponse écrite (dans un ADR ou équivalent) à la question "pourquoi", pas seulement "comment".

---

## Ordre recommandé

1. ✅ **GitHub Action** — fait
2. ✅ **CI multi-plateforme** — fait
3. **Marketplace de plugins** — après un audit de conception dédié (format Phase 4)
4. **Extension VSCode** — après avoir tranché wrapper léger vs réimplémentation
5. **SaaS optionnel** — après avoir répondu à "pourquoi", pas avant