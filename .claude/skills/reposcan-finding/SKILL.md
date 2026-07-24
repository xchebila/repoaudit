---
name: reposcan-finding
description: >
  Defines the Finding struct, severity scoring rules, and quality bar for
  RepoScan security findings. Use when creating or modifying an analyzer,
  writing a new detection rule, adding a Finding, working on the scoring
  engine, or reviewing output from secrets/docker/ci/dependencies/git-history
  analyzers.
---

# RepoScan — Finding format & scoring rules

Ce fichier fait autorité sur la forme de chaque `Finding` produit par un analyzer, et sur la façon dont il pèse dans le score global. Toute nouvelle règle de détection (secrets, docker, CI, dependencies, git-history, code-smells) doit produire des `Finding` conformes à ce contrat, sans exception.

## Principe non négociable

Un `Finding` sans message explicatif clair et sans piste de correction n'est pas un `Finding` valide — c'est du bruit, et RepoScan existe précisément pour l'éviter (voir vision.md, principe "Signal > bruit" et "Explicable").

## Struct Go

```go
type Severity string

const (
    Critical Severity = "CRITICAL" // secret exposé, leak actif
    High     Severity = "HIGH"     // misconfiguration exploitable
    Medium   Severity = "MEDIUM"
    Low      Severity = "LOW"
)

type Finding struct {
    ID          string   // identifiant stable de la règle, ex: "secrets.aws_key"
    Severity    Severity
    Title       string   // résumé en une ligne, affiché en CLI
    Message     string   // pourquoi c'est dangereux — obligatoire, non vide
    Fix         string   // comment corriger — obligatoire, non vide
    File        string   // chemin relatif du fichier concerné
    Line        int      // ligne concernée, 0 si non applicable (ex: git history)
    CommitHash  string   // optionnel, rempli par git-history analyzer
    Category    string   // "secrets" | "docker" | "ci" | "dependencies" | "git-history" | "code-smells"
    Context     string   // hint de triage optionnel, ex: "path looks like a test/fixture directory" — n'affecte jamais Severity ni le score (voir docs/decisions/0001-test-fixture-context.md)
}
```

**Règle stricte** : si `Message` ou `Fix` est vide au moment de la création d'un `Finding`, c'est un bug d'analyzer, pas un détail à corriger plus tard. Ne jamais laisser passer un `Finding` incomplet, même en brouillon de développement.

**`Context` n'est jamais un levier de sévérité.** Un pattern de chemin (`testdata/`, `fixtures/`...) est un indice de triage pour l'utilisateur, pas une preuve d'innocuité — voir `docs/decisions/0001-test-fixture-context.md` pour le raisonnement complet. N'écris jamais de logique du type "si le chemin ressemble à un test, baisser la Severity".

## Barème de scoring

Aligné sur le principe du vision.md : **un `Finding` critique doit dominer le score, jamais s'additionner à égalité avec des `Finding` mineurs.**

| Severity | Impact score |
|---|---|
| CRITICAL | -40 à -100 |
| HIGH | -15 à -40 |
| MEDIUM | -5 à -15 |
| LOW | -1 à -5 |

Quand tu écris ou modifies le scoring engine : ne jamais revenir silencieusement à une moyenne pondérée simple (ex: `total / nombre_de_categories`) sous prétexte de simplicité. Si un changement d'implémentation modifie ce comportement, le signaler explicitement avant de committer.

## Exemples

### ✅ Bon Finding

```go
Finding{
    ID:       "secrets.aws_access_key",
    Severity: Critical,
    Title:    "AWS Access Key exposed",
    Message:  "An AWS access key was found hardcoded in this file. If committed to a public or shared repo, it can be used immediately to access your AWS account.",
    Fix:      "Revoke this key in the AWS IAM console, then move credentials to environment variables or a secrets manager (e.g. AWS Secrets Manager, Vault).",
    File:     "config/settings.py",
    Line:     14,
    Category: "secrets",
}
```
Pourquoi c'est bon : sévérité alignée sur le risque réel, message explique la conséquence concrète (pas juste "c'est mal"), fix est actionnable immédiatement.

### ❌ Mauvais Finding

```go
Finding{
    ID:       "docker.tag",
    Severity: Critical, // ❌ sévérité gonflée
    Title:    "Bad practice detected",
    Message:  "This is not recommended.", // ❌ n'explique rien
    Fix:      "", // ❌ vide, ne doit jamais arriver
    Category: "docker",
}
```
Pourquoi c'est mauvais : un tag `latest` en Dockerfile n'est pas un CRITICAL (aucune fuite de données, aucun accès immédiat compromis) — ça devrait être LOW ou MEDIUM. Le message ne dit ni pourquoi ni comment corriger. C'est exactement le type de finding que le principe "signal > bruit" du vision.md doit éliminer avant qu'il n'atteigne l'utilisateur.

## Checklist avant de merger une nouvelle règle de détection

1. La sévérité reflète-t-elle un risque réel (fuite/accès immédiat = Critical), pas juste "c'est une mauvaise pratique" ?
2. `Message` explique-t-il la conséquence concrète, pas juste qu'un pattern a matché ?
3. `Fix` est-il actionnable en une action claire, pas un renvoi vague à "voir la documentation" ?
4. As-tu un exemple de faux positif plausible pour cette règle ? Si oui, note-le en commentaire dans le code de l'analyzer — ça sert de garde-fou pour les futures modifications de la règle.