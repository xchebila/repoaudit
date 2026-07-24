# Documentation

**Pour utiliser RepoScan, le [README principal](../README.md) suffit** — installation, commandes, exemples de sortie.

Ce qui suit documente le *pourquoi* derrière chaque décision d'architecture, au fil de la construction du projet. Utile si tu es curieux d'un choix précis ou si tu contribues au projet — pas nécessaire pour s'en servir.

## Vue d'ensemble

- [`vision.md`](vision.md) — pitch, philosophie, non-goals, roadmap Phases 1-5 (close, v1.0)
- [`roadmap-long-term.md`](roadmap-long-term.md) — directions post-v1.0 (GitHub Action, CI multi-plateforme, Homebrew : faites ; marketplace de plugins, extension VSCode : en pause faute de besoin réel identifié)

## Comment le projet est testé

- [`testing.md`](testing.md) — corpus de test, critères de sortie mesurables
- [`benchmarks.md`](benchmarks.md) — historique des mesures de perf, phase par phase

## Contrats techniques

- [`plugin-protocol.md`](plugin-protocol.md) — protocole JSON du système de plugins (process séparé, n'importe quel langage)
- [`ci-integrations.md`](ci-integrations.md) — snippets GitLab CI / Jenkins (non testés en conditions réelles — voir l'avertissement en tête du fichier)
- [`examples/reference-plugin.py`](examples/reference-plugin.py) — plugin de référence en Python

## Décisions d'architecture (ADR)

Chaque décision non triviale du projet est documentée au moment où elle est prise, pas reconstruite après coup. Voir [`decisions/`](decisions/) pour la liste complète, dans l'ordre chronologique.
