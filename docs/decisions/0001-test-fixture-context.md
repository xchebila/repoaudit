# ADR 0001 — Ne pas baisser la sévérité sur les chemins test/fixture

## Statut

Accepté (2026-07-02).

## Contexte

En validant le critère de sortie du MVP (scan < 5s, zéro faux positif majeur sur ~20 repos publics), la majorité des findings `secrets.private_key_block` restants après correction des vrais faux positifs (cf. corpus de test) pointaient vers des clés PEM complètes et valides commitées dans des dossiers `testdata/`, `tests/`, `fixtures/` — des certificats de test générés pour des serveurs TLS locaux, pas des secrets de production.

Techniquement, ce ne sont pas des faux positifs : le contenu détecté est bien une clé privée réelle. La question posée était de UX/scoring : faut-il désamorcer l'alerte quand le chemin ressemble à un dossier de test, pour éviter que RepoScan crie systématiquement au loup sur des repos matures qui suivent cette convention ?

## Décision

**Ne jamais baisser automatiquement la sévérité d'un `Finding` sur la seule base d'un pattern de chemin** (`test/`, `tests/`, `testdata/`, `fixture/`, `fixtures/`). Un secret réel déguisé en fichier de test (nom de dossier choisi précisément pour passer inaperçu) resterait un secret réel — le path est un signal de contexte, jamais une preuve d'innocuité.

À la place, `core.Finding` gagne un champ `Context string` (voir `core/finding.go`) : un hint non-autoritatif affiché en CLI ("path looks like a test/fixture directory — verify..."), rempli via `core.LooksLikeTestPath()` (voir `core/testpath.go`). Il n'entre dans aucun calcul de score (`core/scoring.go` l'ignore complètement) et ne change jamais `Severity`.

## Conséquences

- Le scoring reste fidèle au principe du vision.md : un secret exposé domine le score, quel que soit son emplacement.
- L'utilisateur garde un signal de triage rapide en CLI sans que RepoScan se prononce à sa place sur la légitimité du finding.
- Toute future analyzer (docker, ci, git-history) peut réutiliser `core.LooksLikeTestPath()` pour la même annotation contextuelle, sans dupliquer la logique de détection de chemin.
- Ce choix est délibérément conservateur : il accepte plus de bruit visuel (les CRITICAL de fixtures de test restent des CRITICAL) en échange de zéro risque de masquer une vraie fuite. Ne pas revenir dessus sans re-discuter explicitement le compromis.
