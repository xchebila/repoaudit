# 🛡️ RepoAudit — Repository Security Auditor

## 📌 Pitch en une phrase

> **RepoAudit does not analyze code quality. It detects real-world security mistakes that leak data or break production.**

Version plus courte, pour un README ou un tagline GitHub :

> RepoAudit is a 10-second security sanity check for Git repositories.

Ces deux formulations font ce que "git status de la sécurité" ne faisait pas complètement : elles excluent explicitement SonarQube du champ de comparaison au lieu de laisser le lecteur faire le rapprochement lui-même.

## 🧭 Comment RepoAudit doit se ressentir

- Lancé sur n'importe quel repo, sans setup
- Résultat en quelques secondes
- Compréhensible sans lire la doc
- Chaque finding est actionnable immédiatement

Si une feature contredit un de ces quatre points, elle est probablement hors scope (voir Non-Goals ci-dessous).

## 🎯 Pour qui ?

| Persona | Besoin | Moment d'usage |
|---|---|---|
| Dev solo / petite équipe | Vérifier vite qu'un repo n'a pas de fuite avant de push/open-source | `pre-commit`, avant de publier un repo |
| Lead tech / DevOps | Gate de sécurité simple en CI, sans configurer un outil enterprise | Pipeline CI/CD |
| Mainteneur open source | Rassurer les contributeurs et utilisateurs sur la santé du repo | Badge de score dans le README |

Sans persona clair, un outil "développeur" finit vite comme un outil "pour personne". Ça vaut le coup de trancher lequel de ces trois tu sers en premier — probablement le dev solo, vu ton MVP.

## 🧠 Philosophie (inchangée, c'est le cœur du projet)

1. **Signal > bruit** — pas 500 warnings, seulement de l'actionnable.
2. **Explicable** — chaque finding dit *pourquoi* c'est dangereux et *comment* corriger.
3. **Extensible** — core minimal, règles en plugins.

## 🔥 Différenciation

| Outil | Objectif | Limite |
|---|---|---|
| Gitleaks | Secrets uniquement | Pas de score global, pas d'autres catégories |
| SonarQube | Analyse statique complète | Lourd à installer/configurer, pensé pour l'entreprise |
| Snyk | Vulnérabilités de dépendances | Payant à l'échelle, pas centré repo-santé |
| **RepoAudit** | **Health check rapide et actionnable** | À prouver : doit éviter de devenir "SonarQube v2" |

**Le risque principal du projet** : la roadmap (v0.1 à v0.6) couvre à peu près tout ce que fait SonarQube. Le vrai différenciateur n'est pas la liste de checks, c'est l'UX : vitesse, lisibilité, zéro config. Garder ça comme boussole à chaque feature ajoutée — si une feature ralentit le scan ou complexifie la config, elle va à l'encontre de la promesse.

### 🚫 Non-Goals

Sans cette section, la roadmap dérive naturellement vers "mini SonarQube" — chaque nouvelle catégorie de check ressemble à une feature légitime prise isolément, mais l'addition finit par recréer l'outil qu'on voulait éviter. Ces exclusions doivent être aussi visibles que la roadmap elle-même :

RepoAudit ne cherchera **pas** à :

- Remplacer SonarQube ou les outils SAST
- Fournir une analyse statique profonde (AST, dataflow)
- Détecter chaque vulnérabilité possible
- Viser zéro faux positif à tout prix
- Exiger une configuration complexe

**Règle d'arbitrage** : si une analyse est lente, bruyante ou ambiguë, elle est hors scope — même si elle est techniquement faisable.

## 🏗️ Architecture globale

```
repoaudit
│
├── core/            → scanner engine, git reader, report generator, scoring engine
├── analyzers/        → secrets, git-history, dependencies, docker, ci, code-smells
├── plugins/          → règles externes
├── cli/              → commandes
└── output/           → cli, json, html
```

## ⚙️ Roadmap produit

### Phase 1 — MVP : Secrets Scanner (1–2 semaines)
Détection : `.env` committé, clés AWS, tokens GitHub, clés privées (`.pem`, `.key`, `id_rsa`), tokens Stripe/Slack/Discord/OpenAI, JWT brut, secrets dans `.yaml`/`.json`/`.env`/`.config`.
Features : scan fichiers, règles regex, respect du `.gitignore`, output CLI simple, score basique.

**Critère de sortie du MVP** : capable de scanner un repo réel en < 5 secondes avec zéro faux positif majeur sur un set de test de ~20 repos publics connus. Sans ce critère chiffré, "MVP" reste flou.

### Phase 2 — Git History + Docker
- Git History Analyzer : secrets supprimés mais toujours présents dans l'historique (commits, branches supprimées, hash associé).
- Docker Analyzer : `USER root`, tags `latest`, `ADD .` au lieu de `COPY`, secrets dans `ENV`, absence d'utilisateur non-root.

### Phase 3 — Dependencies + CI/CD
- Dependency Scanner via OSV API et GitHub Advisory DB (Go, Python, puis Node en option).
- CI/CD Analyzer : permissions trop larges (`write-all`), actions non pinées (`@main`), secrets exposés dans les workflows, absence de Dependabot.
- **Security Diff Mode** — la feature qui change le positionnement de "scanner" à "outil de review sécurité" :

```
repoaudit diff main feature-branch
```

```
❌ NEW: GitHub token introduced
⚠️ NEW: Dockerfile now runs as root
✔️ FIXED: .env removed from repo
```

C'est particulièrement fort en CI/CD sur une pull request : au lieu d'un score statique du repo entier, on montre exactement ce que *cette* PR introduit ou corrige. C'est le genre de feature qui donne une raison concrète d'ajouter RepoAudit à un pipeline plutôt que de le lancer une fois et d'oublier.

### Phase 4 — Plugin System
Interface minimale :
```go
type Analyzer interface {
    Name() string
    Run(repo RepoContext) []Finding
}
```
Plugins externes envisageables : Terraform, manifests Kubernetes, analyse statique Python, règles enterprise custom.

### Phase 5 — Reporting v1.0
- Outputs : CLI coloré, JSON machine-readable, dashboard HTML.
- Contenu : score de sécurité (0–100), breakdown par catégorie, liste des findings, niveaux de sévérité.

## 📊 Scoring système

```
Secrets           10/10
Git History        7/10
Docker              6/10
Dependencies        8/10
CI/CD               9/10
Code Safety         7/10

TOTAL SCORE: 78/100
GRADE: B
```

### ⚠️ Principe de scoring

**Un seul problème critique doit dominer le score, pas s'additionner comme un problème mineur parmi d'autres.** Un secret exposé n'est pas "10 points en moins comme un `latest` tag Docker" — c'est un incident, et le score doit le refléter immédiatement.

Modèle indicatif à affiner en Phase 5, mais posé dès maintenant comme principe :

- 🔴 Critical (secret exposé, leak) → -40 à -100
- 🟠 High (misconfiguration exploitable) → -15 à -40
- 🟡 Medium → -5 à -15
- 🟢 Low → -1 à -5

Sans cette hiérarchie explicite, un repo avec un secret AWS exposé mais peu d'autres findings pourrait afficher un score correct — ce qui détruirait la crédibilité de l'outil dès le premier vrai incident détecté.

## 🖥️ CLI design

```
repoaudit scan .
repoaudit scan https://github.com/user/repo

repoaudit report --format html
repoaudit report --format json

repoaudit plugins list
repoaudit plugins install xyz
```

## 📄 Exemple de sortie

```
❌ HIGH   - GitHub Token detected in commit a83f1c
⚠️ MEDIUM - Docker runs as root
⚠️ MEDIUM - requirements.txt contains vulnerable dependency
✔️ OK     - No secrets in working tree
```

## 🧾 Configuration

```yaml
# .repoaudit.yml
score:
  threshold: 70

scan:
  git_history: true
  dependencies: true

ignore:
  - node_modules
  - vendor

plugins:
  - secrets
  - docker
  - ci
```

## 🧱 Stack technique

- **Go** — bon choix pour un CLI rapide, binaire unique, facile à distribuer
- CLI : Cobra ou urfave/cli
- Git : go-git
- YAML (CI/CD Analyzer, Phase 3) : gopkg.in/yaml.v3 — pas de parseur YAML en stdlib, lib de référence de l'écosystème Go
- Parallélisation : goroutines
- Output : templating + JSON
- HTTP : OSV / GitHub API

## 🌍 Vision long terme

- GitHub Action officiel : `uses: repoaudit/action@v1`
- Extension VSCode
- Marketplace de plugins
- Intégration CI multi-plateforme (GitLab, Jenkins)
- SaaS optionnel, non obligatoire

## 🧠 Positionnement mental

Ne pense pas : *"je fais un scanner de sécurité."*
Pense : *"je fais un outil de health check de sécurité pour développeurs."*

---

## ✍️ Historique des révisions

**V2 (cette version)**
- Pitch réécrit pour exclure explicitement la comparaison SonarQube plutôt que de la laisser implicite
- Section **Non-Goals** ajoutée — le garde-fou le plus important contre la dérive "mini SonarQube"
- **Security Diff Mode** ajouté en Phase 3 : change le positionnement de scanner ponctuel à outil de review, particulièrement fort en CI/CD sur PR
- Principe de scoring cadré : un critique doit dominer le score, pas s'additionner à égalité avec des mineurs
- Section "Comment RepoAudit doit se ressentir" ajoutée en haut, comme boussole UX

**V1**
- Personas explicites (dev solo / lead tech / mainteneur OSS)
- Risque de positionnement nommé (roadmap = périmètre SonarQube)
- Critère de sortie du MVP chiffré
- Reste (philosophie, architecture, stack, CLI design) inchangé — déjà solide dans le jet original