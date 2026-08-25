# Changelog

All notable changes to this project will be documented in this file.

## [0.2.2] - 2026-08-24

### Miscellaneous

- **deps:** Bump the all-go group across 1 directory with 2 updates ([e0cb8fa](https://github.com/postfinance/chaosmonkey/commit/e0cb8fa17910316058fa3f375df09efd20d127a5))
- **deps:** Bump the actions group across 1 directory with 3 updates ([c630061](https://github.com/postfinance/chaosmonkey/commit/c630061b94390507a26546f4c318eda3a7702652))
- **deps:** Bump the k8s group with 3 updates ([6f8b3ec](https://github.com/postfinance/chaosmonkey/commit/6f8b3ec4d045401e8c450ef907b1ebee959432dd))

## [0.2.1] - 2026-06-03

### Bug Fixes

- Keep all upcoming pods in memory ([ad467e3](https://github.com/postfinance/chaosmonkey/commit/ad467e38a15e767e159294604fee3a7ead909172))

## [0.2.0] - 2026-06-01

### Bug Fixes

- **chart:** Use appVersion as default image tag in cronjob ([715c675](https://github.com/postfinance/chaosmonkey/commit/715c6754b61e1ec415a7626f53efe77473f6c971))
- Exclude static pods ([5ea3c59](https://github.com/postfinance/chaosmonkey/commit/5ea3c59a9eb9b8a8216f9ab9fbdbbf1ec43cf51e))

### Features

- Rework dead man's switch and exclude static pods ([624304c](https://github.com/postfinance/chaosmonkey/commit/624304c65ab9bf6c4c576a4cbd26f349bb436b9a))

### Miscellaneous

- Helm chart improvements ([144da43](https://github.com/postfinance/chaosmonkey/commit/144da436ffb641e902b564f967e6e332b0bf9e92))

### Refactor

- **metrics:** Consolidate pod and kill metrics ([2f52e40](https://github.com/postfinance/chaosmonkey/commit/2f52e4085330b3c6dacf075876787da971f1a488))

## [0.1.1] - 2026-05-29

### Bug Fixes

- **chart:** Use appVersion as default image tag ([9c0c356](https://github.com/postfinance/chaosmonkey/commit/9c0c356000e6b0f118ba7fdb1ac7a5780953fb69))

## [0.1.0] - 2026-05-29

### CI

- Install git-cliff before goreleaser runs ([cd8a41c](https://github.com/postfinance/chaosmonkey/commit/cd8a41ce83941fa7363b0746dc212d8e9a0abd1f))
- Use git-cliff to generate release notes for goreleaser ([47033a6](https://github.com/postfinance/chaosmonkey/commit/47033a6988eb464a59101c7b647bc03dfd89ae80))
- Write release notes to /tmp to avoid dirty git state ([c511d42](https://github.com/postfinance/chaosmonkey/commit/c511d421cc575a126bc355adadf97cf510c58b03))

