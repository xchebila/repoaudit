# 🌍 Roadmap long terme — RepoAudit

Ce document couvre ce qui vient après le v1.0 (Phases 1-5, voir `vision.md`).

**Différence de nature avec vision.md** : les Phases 1-5 avaient chacune un scope technique clair, un critère de sortie mesurable, et un ordre imposé par les dépendances entre elles. Rien ici n'a ce niveau de certitude au départ — ce sont des directions, pas des engagements. Deux items (GitHub Action, CI multi-plateforme) étaient assez mûrs pour être cadrés comme de vraies phases, et sont maintenant faits. Le marketplace de plugins est en pause, faute de besoin réel identifié — pas juste "pas encore audité", une distinction volontaire pour ne pas laisser croire qu'un audit de conception est simplement la prochaine étape logique. Les deux restants (extension VSCode, SaaS) restent au niveau "direction + risque à trancher avant de commencer", pour ne pas donner une fausse impression de planning détaillé sur des sujets encore ouverts.

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

## ⏸️ En pause — en attente d'un besoin réel

### Marketplace de plugins

**Statut** : en pause, pas "à faire". Pas d'audit de conception lancé, faute de besoin identifié.

**Pourquoi la pause plutôt qu'un audit** : le protocole d'isolation (Phase 4) a été construit pour que le mainteneur (ou un contributeur) écrive ses propres règles — jamais dans un but de découverte/publication par des tiers ("découverte/installation de plugins" a d'ailleurs été explicitement mis hors scope lors de l'audit Phase 4, ADR 0008). Rien depuis n'est venu d'un vrai besoin ("quelqu'un veut publier un plugin") — c'est une direction anticipée, pas une demande. Un marketplace introduit en plus le risque le plus sérieux de toute cette roadmap : une question de confiance sur du code découvert et installé (signature, review, sandboxing du marketplace lui-même), que le protocole d'exécution de Phase 4 ne résout pas du tout — l'isoler protège contre un plugin buggé ou malveillant *une fois lancé*, pas contre la découverte d'un plugin déjà compromis. Construire cette surface de risque sans utilisateur en face serait aller à l'encontre du principe déjà appliqué partout ailleurs dans ce projet : ne pas construire avant que ce soit nécessaire (cf. Non-Goals, vision.md).

**Condition de sortie de pause** : un vrai signal d'usage — quelqu'un qui veut publier un plugin, ou un cas d'usage concret remonté. Le jour où ce signal existe, repasser par le même format que l'audit Phase 4 : questions posées explicitement, réponses vérifiées empiriquement quand possible, décision actée dans un ADR avant la première ligne de code.

---

## 🧭 Directions à auditer avant de coder

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
3. ⏸️ **Marketplace de plugins** — en pause, en attente d'un besoin réel identifié (pas un audit de conception en cours)
4. **Extension VSCode** — après avoir tranché wrapper léger vs réimplémentation
5. **SaaS optionnel** — après avoir répondu à "pourquoi", pas avant