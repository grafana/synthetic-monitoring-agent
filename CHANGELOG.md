# Changelog

## [0.34.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.33.0...v0.34.0) (2025-02-11)


### Features

* k6runner: default K6_BROWSER_LOG to info ([287ccda](https://github.com/grafana/synthetic-monitoring-agent/commit/287ccdad739852308fde36e2e79c9abcbd52c899))
* Run agent + chromium as non-root user ([#1187](https://github.com/grafana/synthetic-monitoring-agent/issues/1187)) ([96667da](https://github.com/grafana/synthetic-monitoring-agent/commit/96667da3ca45a6746ea5bc5edbd31437a304a0db))
* update proto to include secret url and location ([#1192](https://github.com/grafana/synthetic-monitoring-agent/issues/1192)) ([a0ef302](https://github.com/grafana/synthetic-monitoring-agent/commit/a0ef302588b4cf5209534f70f4e634db5d4b2195))


### Fixes

* increase Scripted and Browser max timeout to 180s ([ecb198a](https://github.com/grafana/synthetic-monitoring-agent/commit/ecb198aa4be5ab7b923d5b7759886f8774c6f043))
* increase Scripted and Browser max timeout to 180s ([#1173](https://github.com/grafana/synthetic-monitoring-agent/issues/1173)) ([ecb198a](https://github.com/grafana/synthetic-monitoring-agent/commit/ecb198aa4be5ab7b923d5b7759886f8774c6f043))
* k6runner/local: disable k6 api server ([3a9439e](https://github.com/grafana/synthetic-monitoring-agent/commit/3a9439e3da7b02a67862a0f0d1d3ac6a4390ae7a))
* Point CODEOWNERS to synthetic-monitoring-be ([#1180](https://github.com/grafana/synthetic-monitoring-agent/issues/1180)) ([415a084](https://github.com/grafana/synthetic-monitoring-agent/commit/415a084a47da42cc8ea3a040453b831a568a61ab))
* tag docker images with the bare version ([#1178](https://github.com/grafana/synthetic-monitoring-agent/issues/1178)) ([e39b576](https://github.com/grafana/synthetic-monitoring-agent/commit/e39b576116c0c43e9f8b393dda2c3f0c888f47d1))


### Miscellaneous Chores

* remove xk6 leftovers ([c8d3a7e](https://github.com/grafana/synthetic-monitoring-agent/commit/c8d3a7eed1bce36eeb1ba5117c0befc662417cfd))
* Revert "Run agent + chromium as non-root user ([#965](https://github.com/grafana/synthetic-monitoring-agent/issues/965))" ([#1186](https://github.com/grafana/synthetic-monitoring-agent/issues/1186)) ([44a7bde](https://github.com/grafana/synthetic-monitoring-agent/commit/44a7bde4e9b8b70fad0ba26de765f73090b3af29))
* Update actions/create-github-app-token digest to 67e27a7 ([#1177](https://github.com/grafana/synthetic-monitoring-agent/issues/1177)) ([2fe64fc](https://github.com/grafana/synthetic-monitoring-agent/commit/2fe64fc8688b9f0ba0da218639f6b40349e9bd98))
* Update docker/setup-buildx-action action to v3.9.0 ([#1188](https://github.com/grafana/synthetic-monitoring-agent/issues/1188)) ([aace0e4](https://github.com/grafana/synthetic-monitoring-agent/commit/aace0e428fae7addcc529bcb066c3e45aa0ee549))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.38.1 ([#1189](https://github.com/grafana/synthetic-monitoring-agent/issues/1189)) ([1d039aa](https://github.com/grafana/synthetic-monitoring-agent/commit/1d039aa3df3b560a5310a9ca1ff7a9a66266c792))
* update logo and screenshot ([#1176](https://github.com/grafana/synthetic-monitoring-agent/issues/1176)) ([f89f2bb](https://github.com/grafana/synthetic-monitoring-agent/commit/f89f2bbcefafa5409d65d742857db139afa7132c))
* Update module github.com/golangci/golangci-lint to v1.63.4 ([02b7388](https://github.com/grafana/synthetic-monitoring-agent/commit/02b73887bb0aaab79b958cf21e4890d3fe5edb11))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.16.0 ([#1164](https://github.com/grafana/synthetic-monitoring-agent/issues/1164)) ([d086821](https://github.com/grafana/synthetic-monitoring-agent/commit/d086821fc80e0474be71300938cb6b61672e7565))
* Update module github.com/prometheus-community/pro-bing to v0.6.1 ([#1182](https://github.com/grafana/synthetic-monitoring-agent/issues/1182)) ([89628cd](https://github.com/grafana/synthetic-monitoring-agent/commit/89628cd9b0340d75730a9abdd62586b0f61636e9))
* Update module golang.org/x/net to v0.35.0 ([#1195](https://github.com/grafana/synthetic-monitoring-agent/issues/1195)) ([5b4276b](https://github.com/grafana/synthetic-monitoring-agent/commit/5b4276bf5509f44958a6671f53a188b3baad8e36))
* Update module golang.org/x/sync to v0.11.0 ([#1183](https://github.com/grafana/synthetic-monitoring-agent/issues/1183)) ([a97765f](https://github.com/grafana/synthetic-monitoring-agent/commit/a97765f6f6e2f4dbf2bfc9b2188e8eabc3934662))
* Update module google.golang.org/grpc to v1.70.0 ([#1174](https://github.com/grafana/synthetic-monitoring-agent/issues/1174)) ([3a9ba62](https://github.com/grafana/synthetic-monitoring-agent/commit/3a9ba6279d9cf7cd6a3a49a4f7f5f1958a83d477))
* Update prometheus-go ([#1044](https://github.com/grafana/synthetic-monitoring-agent/issues/1044)) ([eb02887](https://github.com/grafana/synthetic-monitoring-agent/commit/eb02887b4dbd3a11976f96299ee90d4967326082))

## [0.33.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.32.0...v0.33.0) (2025-01-29)


### Features

* Replace go-ping with pro-bing and enable DF ([#1167](https://github.com/grafana/synthetic-monitoring-agent/issues/1167)) ([934ba8e](https://github.com/grafana/synthetic-monitoring-agent/commit/934ba8e851aa5c0b782e9c5d546c1b8a72f5877d))


### Fixes

* Tag images with the bare version. ([#1166](https://github.com/grafana/synthetic-monitoring-agent/issues/1166)) ([b6ef348](https://github.com/grafana/synthetic-monitoring-agent/commit/b6ef348badf7951b5d2dca34dd38394927c9f5fd))
* Use the recommended 'persist-credentials: false' setting ([#1143](https://github.com/grafana/synthetic-monitoring-agent/issues/1143)) ([270f956](https://github.com/grafana/synthetic-monitoring-agent/commit/270f956e52efc6fe772c162220b43874b62372fa))


### Miscellaneous Chores

* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.37.1 ([#1171](https://github.com/grafana/synthetic-monitoring-agent/issues/1171)) ([5ecf37f](https://github.com/grafana/synthetic-monitoring-agent/commit/5ecf37fe5666c0d144a033e886e17c302e3c434c))
* Update module github.com/miekg/dns to v1.1.63 ([#1163](https://github.com/grafana/synthetic-monitoring-agent/issues/1163)) ([f0810fc](https://github.com/grafana/synthetic-monitoring-agent/commit/f0810fc2751faf698cb694f8dbd996110596dcf2))
* Update module github.com/prometheus-community/pro-bing to v0.6.0 ([#1170](https://github.com/grafana/synthetic-monitoring-agent/issues/1170)) ([4753f6f](https://github.com/grafana/synthetic-monitoring-agent/commit/4753f6f867c7c1ccc0de182e1adde21bcf9a916d))
* Update module github.com/prometheus/prometheus to v0.55.1 ([#980](https://github.com/grafana/synthetic-monitoring-agent/issues/980)) ([17d6dc2](https://github.com/grafana/synthetic-monitoring-agent/commit/17d6dc280090ec534fd0f7c228f4183c9414b72e))
* Update module github.com/spf13/afero to v1.12.0 ([#1172](https://github.com/grafana/synthetic-monitoring-agent/issues/1172)) ([3e92990](https://github.com/grafana/synthetic-monitoring-agent/commit/3e929901dc6a6acdf211cd6aa74212de3896c82d))

## [0.32.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.31.0...v0.32.0) (2025-01-27)


### Features

* fetch precompiled xk6 extension from `grafana/xk6-sm` ([#966](https://github.com/grafana/synthetic-monitoring-agent/issues/966)) ([0a57fad](https://github.com/grafana/synthetic-monitoring-agent/commit/0a57fad38b720d755028abad755652293e8fd451))
* k6runner: improve error handling for k6 output ([7cc7746](https://github.com/grafana/synthetic-monitoring-agent/commit/7cc77469a3a012a03213ed5b59dba2d4bc7526e0))


### Fixes

* increase Scripted and Browser max timeout to 120s ([#1136](https://github.com/grafana/synthetic-monitoring-agent/issues/1136)) ([#1160](https://github.com/grafana/synthetic-monitoring-agent/issues/1160)) ([24e2a41](https://github.com/grafana/synthetic-monitoring-agent/commit/24e2a417407fe196df106dc1c23ec58c0a2857bd))
* Update grafana-build-tools to v0.37.0 ([#1162](https://github.com/grafana/synthetic-monitoring-agent/issues/1162)) ([8ab9470](https://github.com/grafana/synthetic-monitoring-agent/commit/8ab9470d60bbc941f586636f1286d7493735d98e))


### Miscellaneous Chores

* Update actions/checkout action to v4.2.2 ([#1156](https://github.com/grafana/synthetic-monitoring-agent/issues/1156)) ([9d2705d](https://github.com/grafana/synthetic-monitoring-agent/commit/9d2705dddc3bcd251a211f467d14ec310e022eb8))
* Update actions/setup-go action to v5.3.0 ([#1157](https://github.com/grafana/synthetic-monitoring-agent/issues/1157)) ([21cf1fe](https://github.com/grafana/synthetic-monitoring-agent/commit/21cf1fe4226fb50fa798d9b128ad0073812d9269))
* Update alpine Docker tag to v3.21.2 ([f05b158](https://github.com/grafana/synthetic-monitoring-agent/commit/f05b158d41aafe1d2bb9a40c6fc5071ddac7b492))
* Update docker/build-push-action action to v6.13.0 ([#1158](https://github.com/grafana/synthetic-monitoring-agent/issues/1158)) ([61a4197](https://github.com/grafana/synthetic-monitoring-agent/commit/61a41975939a4e244563a6836fd7a1776407e684))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v131.0.6778.264-r0-3.21.2 ([86da451](https://github.com/grafana/synthetic-monitoring-agent/commit/86da451134e76020a0b825319b51a04616a1bc4c))
* Update module github.com/Antonboom/nilnil to v1.0.1 ([#1149](https://github.com/grafana/synthetic-monitoring-agent/issues/1149)) ([1324150](https://github.com/grafana/synthetic-monitoring-agent/commit/132415079fa288846fafa82fa8f537bd58eccf92))
* Update module github.com/KimMachineGun/automemlimit to v0.7.0 ([#1141](https://github.com/grafana/synthetic-monitoring-agent/issues/1141)) ([24c91b2](https://github.com/grafana/synthetic-monitoring-agent/commit/24c91b23dbff06ae894c6f67c91fc269f365a6e0))

## [0.31.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.30.2...v0.31.0) (2025-01-15)


### Features

* Add policy bot configuration ([#1144](https://github.com/grafana/synthetic-monitoring-agent/issues/1144)) ([146f642](https://github.com/grafana/synthetic-monitoring-agent/commit/146f64207eed8a7c13bbb1c0fbfe94854d090cb6))


### Fixes

* increase Scripted and Browser max timeout to 90s ([#1136](https://github.com/grafana/synthetic-monitoring-agent/issues/1136)) ([8ef7d2a](https://github.com/grafana/synthetic-monitoring-agent/commit/8ef7d2a51db161fedcdd3cd2a60ca37d59d89815))
* Publish images to docker hub ([#1145](https://github.com/grafana/synthetic-monitoring-agent/issues/1145)) ([bcd2008](https://github.com/grafana/synthetic-monitoring-agent/commit/bcd2008f369dd43e2492adce9d750b11426fbaed)), closes [#1132](https://github.com/grafana/synthetic-monitoring-agent/issues/1132)


### Miscellaneous Chores

* Update actions/create-github-app-token digest to c1a2851 ([#1135](https://github.com/grafana/synthetic-monitoring-agent/issues/1135)) ([dfc1fd4](https://github.com/grafana/synthetic-monitoring-agent/commit/dfc1fd41ab0d728333151b38294776509d45794e))
* Update actions/upload-artifact digest to 65c4c4a ([#1127](https://github.com/grafana/synthetic-monitoring-agent/issues/1127)) ([28126e9](https://github.com/grafana/synthetic-monitoring-agent/commit/28126e9c3478e6c58e5884b8047025f991ae2e7e))
* Update docker/build-push-action action to v6.11.0 ([#1139](https://github.com/grafana/synthetic-monitoring-agent/issues/1139)) ([c190dad](https://github.com/grafana/synthetic-monitoring-agent/commit/c190dad5c6b27df93050f6cef3c3aa125ee54f75))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.36.0 ([#1140](https://github.com/grafana/synthetic-monitoring-agent/issues/1140)) ([ead4b9f](https://github.com/grafana/synthetic-monitoring-agent/commit/ead4b9f50f1557b79e4631e968cafc26cd501760))
* Update module golang.org/x/net to v0.33.0 [SECURITY] ([#1142](https://github.com/grafana/synthetic-monitoring-agent/issues/1142)) ([f4f1c5d](https://github.com/grafana/synthetic-monitoring-agent/commit/f4f1c5d3f0a44cb8414fb6faf0742311dc5020bb))
* Update module google.golang.org/grpc to v1.69.4 ([#1138](https://github.com/grafana/synthetic-monitoring-agent/issues/1138)) ([20cc2ff](https://github.com/grafana/synthetic-monitoring-agent/commit/20cc2ff9e29410b4a96498306bb0ade089127b76))

## [0.30.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.30.1...v0.30.2) (2025-01-13)


### Fixes

* Bump golang.org/x/crypto to v0.32.0 ([#1131](https://github.com/grafana/synthetic-monitoring-agent/issues/1131)) ([112bbad](https://github.com/grafana/synthetic-monitoring-agent/commit/112bbad78ce97229f41fd99ce799bbda19c95bdc))


### Miscellaneous Chores

* Update grafana/shared-workflows digest to bec45d4 ([#1130](https://github.com/grafana/synthetic-monitoring-agent/issues/1130)) ([1642853](https://github.com/grafana/synthetic-monitoring-agent/commit/1642853dac54e74a587b792858f256f451f7d91b))
* Update module google.golang.org/grpc to v1.69.2 ([#1128](https://github.com/grafana/synthetic-monitoring-agent/issues/1128)) ([82a293f](https://github.com/grafana/synthetic-monitoring-agent/commit/82a293f38ccce214792157a44a889ae6cff7c22e))

## [0.30.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.30.0...v0.30.1) (2024-12-19)


### Miscellaneous Chores

* Update dependency go to v1.23.4 ([#1095](https://github.com/grafana/synthetic-monitoring-agent/issues/1095)) ([b61444b](https://github.com/grafana/synthetic-monitoring-agent/commit/b61444b386a83ccec48a6b2518f11275d48baffd))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.34.0 ([#1102](https://github.com/grafana/synthetic-monitoring-agent/issues/1102)) ([3c86f3b](https://github.com/grafana/synthetic-monitoring-agent/commit/3c86f3bc3f1b399db233762473bcb2f89fa9e19e))
* Update module golang.org/x/net to v0.33.0 [SECURITY] ([#1129](https://github.com/grafana/synthetic-monitoring-agent/issues/1129)) ([40720bd](https://github.com/grafana/synthetic-monitoring-agent/commit/40720bdc7aa6a7b05900ccf641fafa240425cc86))

## [0.30.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.10...v0.30.0) (2024-12-17)


### Features

* remove drone setup ([1982d52](https://github.com/grafana/synthetic-monitoring-agent/commit/1982d52679db45c4be296bcb1a2769a172c13a08))


### Fixes

* bump minor, not patch, for features ([035c146](https://github.com/grafana/synthetic-monitoring-agent/commit/035c1468c3cbfc348f57aeb7c0c6985a5731641d))
* pass version to argo workflow ([#1105](https://github.com/grafana/synthetic-monitoring-agent/issues/1105)) ([43d9558](https://github.com/grafana/synthetic-monitoring-agent/commit/43d9558dd6e0413c320122188e68fa8552c6bcdf))


### Miscellaneous Chores

* Fix changelog ([#1107](https://github.com/grafana/synthetic-monitoring-agent/issues/1107)) ([2afc7e2](https://github.com/grafana/synthetic-monitoring-agent/commit/2afc7e215a39b79dd9ca219f62a0a217c432e279))
* Format changelog ([#1109](https://github.com/grafana/synthetic-monitoring-agent/issues/1109)) ([48acd4d](https://github.com/grafana/synthetic-monitoring-agent/commit/48acd4de828885ae033fe8d66cb1c735b41febc7))
* rename add err prefix to unsupportedCheckType error ([64b0cb1](https://github.com/grafana/synthetic-monitoring-agent/commit/64b0cb1967a8517df0d21accb51adf8d0f4edafd))
* Set release version ([#1113](https://github.com/grafana/synthetic-monitoring-agent/issues/1113)) ([19de6df](https://github.com/grafana/synthetic-monitoring-agent/commit/19de6df3170c056d40971c86c927f81064b750fa))
* Set release version ([#1119](https://github.com/grafana/synthetic-monitoring-agent/issues/1119)) ([d548f56](https://github.com/grafana/synthetic-monitoring-agent/commit/d548f56d5199d4ea5cfa49e0521213cec5426cf7))
* Update actions/cache action to v4.2.0 ([76681db](https://github.com/grafana/synthetic-monitoring-agent/commit/76681dbf07a0b10faafb429e2374227a4695f6cd))
* Update actions/checkout action to v4.2.2 ([8751eef](https://github.com/grafana/synthetic-monitoring-agent/commit/8751eef420ab4009dc3e2243cc80d948f2356805))
* Update actions/setup-go action to v5.2.0 ([bf1829e](https://github.com/grafana/synthetic-monitoring-agent/commit/bf1829efcbe871a28aeecba78e4db51c6032196f))
* Update alpine Docker tag to v3.21.0 ([20ba3a9](https://github.com/grafana/synthetic-monitoring-agent/commit/20ba3a937de122b606567c5786d9ae45b396c45a))
* Update docker/build-push-action action to v6.10.0 ([004ef45](https://github.com/grafana/synthetic-monitoring-agent/commit/004ef45497cf89922cab56f135e9ced05c21577c))
* Update docker/setup-buildx-action action to v3.8.0 ([1e3831a](https://github.com/grafana/synthetic-monitoring-agent/commit/1e3831a07327d4d2a2f2b3fde43af3eca1ea83af))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v131.0.6778.108-r0-3.21.0 ([6c126df](https://github.com/grafana/synthetic-monitoring-agent/commit/6c126dfe0f052a38a986e8523a76f8628cab1ff4))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v131.0.6778.139-r0-3.21.0 ([74faf88](https://github.com/grafana/synthetic-monitoring-agent/commit/74faf8840dd60d2b936f5594bf26aaf9182ad616))
* Update golang.org/x/exp digest to 4a55095 ([e995923](https://github.com/grafana/synthetic-monitoring-agent/commit/e995923f93e19e3c5306b8d210d883848082434f))
* Update grafana/shared-workflows digest to 4abacd5 ([844daa5](https://github.com/grafana/synthetic-monitoring-agent/commit/844daa502f9a14c3fe2b7e52f640b6e9d1129c1e))
* Update grafana/shared-workflows digest to 5a093ed ([7c1d2ad](https://github.com/grafana/synthetic-monitoring-agent/commit/7c1d2adcceeaa764fca1a5f56ba8ccf355b844ef))
* Update grafana/shared-workflows digest to 5b45f78 ([90caa92](https://github.com/grafana/synthetic-monitoring-agent/commit/90caa929331438ef90a0d3589737ae494110a835))
* Update grafana/shared-workflows digest to a4e8131 ([#1121](https://github.com/grafana/synthetic-monitoring-agent/issues/1121)) ([eb6eefe](https://github.com/grafana/synthetic-monitoring-agent/commit/eb6eefe1a094cb4abeecfc4cc4de45f294497b25))
* Update module golang.org/x/net to v0.32.0 ([e42e7d0](https://github.com/grafana/synthetic-monitoring-agent/commit/e42e7d09c34b05a9d392ca75647b30d69936d556))
* Update module google.golang.org/grpc to v1.68.1 ([8e76cce](https://github.com/grafana/synthetic-monitoring-agent/commit/8e76cce4de854ac46e08e38ffd35d8ed9f62b78b))
* Update module google.golang.org/grpc to v1.69.0 ([f94f827](https://github.com/grafana/synthetic-monitoring-agent/commit/f94f827762215b836cb39f3037690a374dfc1246))

## [0.29.10](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.9...v0.29.10) (2024-12-10)


### Fixes

* increase SHA-1 short version length ([#1092](https://github.com/grafana/synthetic-monitoring-agent/issues/1092)) ([a09f85d](https://github.com/grafana/synthetic-monitoring-agent/commit/a09f85dfc588f674d1fbe1bd0706fc218965d05a))

## [0.29.9](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.8...v0.29.9) (2024-12-05)


### Miscellaneous Chores

* Update actions/checkout digest to 11bd719 ([03f6e2e](https://github.com/grafana/synthetic-monitoring-agent/commit/03f6e2e244812317c700e422db8fb7c82c4a798b))
* Update actions/upload-artifact digest to b4b15b8 ([17502e0](https://github.com/grafana/synthetic-monitoring-agent/commit/17502e0936483ac9e38a91ffa15da8c2910139cd))
* Update golang.org/x/exp digest to 2d47ceb ([07b80c5](https://github.com/grafana/synthetic-monitoring-agent/commit/07b80c5f534a659a6fc0c358c65692feb889f29a))
* Update module kernel.org/pub/linux/libs/security/libcap/cap to v1.2.73 ([d5437a7](https://github.com/grafana/synthetic-monitoring-agent/commit/d5437a7902a12612b1496f6643f1e656783c1061))
* use grafana/sm-renovate shared presets and actions ([de8e948](https://github.com/grafana/synthetic-monitoring-agent/commit/de8e9481888f9196c0089c5400481f7672630e97))


### Fixes

* use `%q` instead of `"%s"` for free quote escaping ([6bfec89](https://github.com/grafana/synthetic-monitoring-agent/commit/6bfec890cf4fe2bf04eaaa1269702b681dd77769))

## [0.29.8](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.7...v0.29.8) (2024-11-27)


### Miscellaneous Chores

* Fix release-please commit case ([46de199](https://github.com/grafana/synthetic-monitoring-agent/commit/46de1991151b9b4904bab41e756f609d47794720))


### Fixes

* pull in newer version of trigger-argo-workflow action ([#1075](https://github.com/grafana/synthetic-monitoring-agent/issues/1075)) ([efb5c44](https://github.com/grafana/synthetic-monitoring-agent/commit/efb5c443c011c547f88e8ecef5179ec9075215da))
* trigger argo release workflow from GHA ([#1074](https://github.com/grafana/synthetic-monitoring-agent/issues/1074)) ([2f45a14](https://github.com/grafana/synthetic-monitoring-agent/commit/2f45a142d1e160d74ca23fa8fe5ccbad19cd4fb7))

## [0.29.7](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.6...v0.29.7) (2024-11-26)


### Miscellaneous Chores

* Dockerfile: build browser image from chromium-swiftshader-alpine ([b8ff6ad](https://github.com/grafana/synthetic-monitoring-agent/commit/b8ff6ad807c35ad09b730f054b923c51b97af285))
* renovate: remove config related to alpine packages ([2aefb4c](https://github.com/grafana/synthetic-monitoring-agent/commit/2aefb4c3fadbc8423d3927117fd30789c6540456))
* renovate: use loose versioning for chromium-swiftshader-alpine image ([82eef25](https://github.com/grafana/synthetic-monitoring-agent/commit/82eef258750a8662c2bd4403cc1ba43073f42516))
* Update module github.com/golangci/golangci-lint to v1.62.2 ([1dc57ad](https://github.com/grafana/synthetic-monitoring-agent/commit/1dc57ad07f8cb6d26cf8f11418582cdb98ddd00a))
* Update module github.com/stretchr/testify to v1.10.0 ([926d2ee](https://github.com/grafana/synthetic-monitoring-agent/commit/926d2eef71170d7c1f20ff145dabd773a8a2d998))

## [0.29.6](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.5...v0.29.6) (2024-11-20)


### Miscellaneous Chores

* Update dependency chromium-swiftshader to v131 ([4c44fa9](https://github.com/grafana/synthetic-monitoring-agent/commit/4c44fa95f2e2701cbef6eafdcc405b0c2edd467c))

## [0.29.5](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.4...v0.29.5) (2024-11-18)


### Fixes

* Do not specify `--vus` or `--iterations` for browser checks ([a23d5fa](https://github.com/grafana/synthetic-monitoring-agent/commit/a23d5fa087718c2cc4c70d740f6c0025c1cafd41))
* use different chromium versions for different architectures ([#1053](https://github.com/grafana/synthetic-monitoring-agent/issues/1053)) ([14b309d](https://github.com/grafana/synthetic-monitoring-agent/commit/14b309d8317369f2dfea60ceee285ea2d6dbf6eb))


### Miscellaneous Chores

* Add support for chore commits in release-please ([#1046](https://github.com/grafana/synthetic-monitoring-agent/issues/1046)) ([807ac78](https://github.com/grafana/synthetic-monitoring-agent/commit/807ac78238953512d5808a55496098eb4f3c20f8))
* change release commit title ([#1039](https://github.com/grafana/synthetic-monitoring-agent/issues/1039)) ([79f6aca](https://github.com/grafana/synthetic-monitoring-agent/commit/79f6acae1a6c4fb2644b5417f6b4abc70da708fa))
* move named anchor in changelog ([#1040](https://github.com/grafana/synthetic-monitoring-agent/issues/1040)) ([c186092](https://github.com/grafana/synthetic-monitoring-agent/commit/c186092687d8bfea14d05f74d70a3938cbd9e02e))
* Throttle renovate updates ([599f0a6](https://github.com/grafana/synthetic-monitoring-agent/commit/599f0a607ebf32fee294d38f3e251c07c71ed00c))
* Update ghcr.io/renovatebot/renovate Docker tag to v39.10.2 ([365693f](https://github.com/grafana/synthetic-monitoring-agent/commit/365693fdfdda667fcf933a610cb6e8440314556d))
* Update ghcr.io/renovatebot/renovate Docker tag to v39.11.7 ([238ec5a](https://github.com/grafana/synthetic-monitoring-agent/commit/238ec5a564b2384d4c4e2b79de04897fc982ebc9))
* Update ghcr.io/renovatebot/renovate Docker tag to v39.14.1 ([522e0d1](https://github.com/grafana/synthetic-monitoring-agent/commit/522e0d184a82e6ac4626c62420dff2b9018af18b))
* Update module github.com/golangci/golangci-lint to v1.62.0 ([138ce6c](https://github.com/grafana/synthetic-monitoring-agent/commit/138ce6c5b6a84f18eca1b7d0a133d31df3ea1b45))

## [0.29.4](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.3...v0.29.4) (2024-11-11)


### Fixes

* add packages to release ([#976](https://github.com/grafana/synthetic-monitoring-agent/issues/976)) ([97ee505](https://github.com/grafana/synthetic-monitoring-agent/commit/97ee5052a24ccac67f65aaac78354da01a172480))
* change vault_instance to ops ([#978](https://github.com/grafana/synthetic-monitoring-agent/issues/978)) ([346a3a0](https://github.com/grafana/synthetic-monitoring-agent/commit/346a3a0f4ea3290f15131d386b3de51cf084e365))
* k6runner: add level error to deferred log reporting code from runner ([dde3046](https://github.com/grafana/synthetic-monitoring-agent/commit/dde3046bfb7b611f9896c2a86a17598a6364ae87))
* simplify TestTenantPusher ([#979](https://github.com/grafana/synthetic-monitoring-agent/issues/979)) ([ae46ff3](https://github.com/grafana/synthetic-monitoring-agent/commit/ae46ff352dbcaae1e0934cc4e954d13e8d2af56c))

## [0.29.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.2...v0.29.3) (2024-11-04)


### Release

* Internal release ([#972](https://github.com/grafana/synthetic-monitoring-agent/issues/972)) ([c5c11ae](https://github.com/grafana/synthetic-monitoring-agent/commit/c5c11ae72b7284786648e87cfd79b8eaa9fdfe97))

## [0.29.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.1...v0.29.2) (2024-11-01)


### Fixes

* rework package creation ([#967](https://github.com/grafana/synthetic-monitoring-agent/issues/967)) ([9ec7359](https://github.com/grafana/synthetic-monitoring-agent/commit/9ec7359e5a799c2421e74fe406a27e178f63df40))

## [0.29.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.29.0...v0.29.1) (2024-10-28)


### Release

* Internal release ([#962](https://github.com/grafana/synthetic-monitoring-agent/issues/962)) ([074c575](https://github.com/grafana/synthetic-monitoring-agent/commit/074c57590d546ab46d2f7497533eeaf77e27b411))

## [0.29.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.28.2...v0.29.0) (2024-10-25)


### Features

* k6runner: add check metadata and type to remote runner requests ([#928](https://github.com/grafana/synthetic-monitoring-agent/issues/928)) ([ce37f32](https://github.com/grafana/synthetic-monitoring-agent/commit/ce37f326c839e57795b7f98beecb593f0a83076a))

## [0.28.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.28.1...v0.28.2) (2024-10-19)


### Fixes

* Drone jsonnet source files ([#937](https://github.com/grafana/synthetic-monitoring-agent/issues/937)) ([1fa3a9d](https://github.com/grafana/synthetic-monitoring-agent/commit/1fa3a9dd2c3beb7a22de12c242c48b935745e0d1))
* **drone:** Resign .drone.yml file ([#935](https://github.com/grafana/synthetic-monitoring-agent/issues/935)) ([8c0bd2f](https://github.com/grafana/synthetic-monitoring-agent/commit/8c0bd2ffd788792aaea6dca2c55a162ff3333ae0))

## [0.28.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.28.0...v0.28.1) (2024-10-01)


### Release

* internal release ([#918](https://github.com/grafana/synthetic-monitoring-agent/issues/918)) ([60727a5](https://github.com/grafana/synthetic-monitoring-agent/commit/60727a54971e13f4f46f90f65db2e4253c9f6e00))

<a name="v0.28.0"></a>
## [v0.28.0] - 2024-09-19
### Feature
- add retries to ICMP prober ([#896](https://github.com/grafana/synthetic-monitoring-agent/issues/896))

### Fix
- allow probers to provide a duration value ([#898](https://github.com/grafana/synthetic-monitoring-agent/issues/898))


<a name="v0.27.0"></a>
## [v0.27.0] - 2024-09-19
### K6runner
- promote log messages surfacing errors to warning level
- error if script timeout is not set

### Scraper
- use check frequency as the context deadline for k6 checks

### Scripts
- update go to 1.23


<a name="v0.26.0"></a>
## [v0.26.0] - 2024-09-02
### Dependabot
- remove

### Dockerfile
- pin hash of debian:stable-slim image ([#828](https://github.com/grafana/synthetic-monitoring-agent/issues/828))

### Drone
- regenerate pipelines

### Feat
- Validate browser capability ([#809](https://github.com/grafana/synthetic-monitoring-agent/issues/809))

### Go
- upgrade to 1.23 ([#838](https://github.com/grafana/synthetic-monitoring-agent/issues/838))

### K6runner
- always log error code and string to user's logger

### Renovate
- add `dependencies` label to PRs
- enable default managers
- group prometheus-go updates
- fix grafana-build-tools dependency regex


<a name="v0.25.2"></a>
## [v0.25.2] - 2024-07-31

<a name="v0.25.1"></a>
## [v0.25.1] - 2024-07-30
### K6runner
- handle ErrorCodeFailed ([#791](https://github.com/grafana/synthetic-monitoring-agent/issues/791))


<a name="v0.25.0"></a>
## [v0.25.0] - 2024-07-15
### Cmd
- default to sm-k6 binary

### Dockerfile
- copy sm-specific k6 as sm-k6 instead of just k6

### Grpc
- nolint deprecated grpc options

### Http
- rename `promconfig.Header` to `promconfig.ProxyHeader`

### K6runner
- log errors encountered by logfmt parser
- send logs even if metrics are malformed


<a name="v0.24.3"></a>
## [v0.24.3] - 2024-06-19
### K6runner
- prevent clearing ip denylist when calling WithLogger
- use non-pointer LocalRunner everywhere
- apply empty IP denylist even if it is empty
- rename Script to Processor

### Prober
- log errors returned by k6-backed probes as errors

### Scraper
- formatting


<a name="v0.24.2"></a>
## [v0.24.2] - 2024-06-13
### Fix
- deprecate --features and warn user ([#726](https://github.com/grafana/synthetic-monitoring-agent/issues/726))
- Interpolate variables into MultiHTTP request bodies ([#713](https://github.com/grafana/synthetic-monitoring-agent/issues/713))

### K6runner
- use check context for http request ([#715](https://github.com/grafana/synthetic-monitoring-agent/issues/715))


<a name="v0.24.1"></a>
## [v0.24.1] - 2024-04-30
### Fix
- report duration from script ([#698](https://github.com/grafana/synthetic-monitoring-agent/issues/698))


<a name="v0.24.0"></a>
## [v0.24.0] - 2024-04-30
### Feature
- automatically set up GOMEMLIMIT ([#691](https://github.com/grafana/synthetic-monitoring-agent/issues/691))

### Fix
- use uniform timeout validation logic ([#693](https://github.com/grafana/synthetic-monitoring-agent/issues/693))
- TestTickWithOffset sometimes if offset is 0 ([#686](https://github.com/grafana/synthetic-monitoring-agent/issues/686))

### K6runner
- inspect errors and propagate unexpected ones to the probe
- handle errors reported by http runners


<a name="v0.23.4"></a>
## [v0.23.4] - 2024-04-17
### Feature
- upgrade k6 to v0.50.0 ([#681](https://github.com/grafana/synthetic-monitoring-agent/issues/681))


<a name="v0.23.3"></a>
## [v0.23.3] - 2024-04-10

<a name="v0.23.2"></a>
## [v0.23.2] - 2024-04-08
### Dependabot
- group prometheus updates ([#664](https://github.com/grafana/synthetic-monitoring-agent/issues/664))


<a name="v0.23.1"></a>
## [v0.23.1] - 2024-03-18

<a name="v0.23.0"></a>
## [v0.23.0] - 2024-03-14
### Experimental
- increase max frequency to 1 hour ([#645](https://github.com/grafana/synthetic-monitoring-agent/issues/645))

### Feature
- switch to pusher v2 by default ([#655](https://github.com/grafana/synthetic-monitoring-agent/issues/655))


<a name="v0.22.0"></a>
## [v0.22.0] - 2024-03-11
### Feature
- allow checks to run less often ([#611](https://github.com/grafana/synthetic-monitoring-agent/issues/611))

### Fix
- telemetry region label ([#638](https://github.com/grafana/synthetic-monitoring-agent/issues/638))


<a name="v0.21.0"></a>
## [v0.21.0] - 2024-02-26
### Feature
- promote adhoc to permanent feature ([#615](https://github.com/grafana/synthetic-monitoring-agent/issues/615))

### Fix
- missing http check regex validations ([#612](https://github.com/grafana/synthetic-monitoring-agent/issues/612))


<a name="v0.20.1"></a>
## [v0.20.1] - 2024-02-12
### Fix
- add test for HTTP check with a long URL


<a name="v0.19.6"></a>
## [v0.19.6] - 2024-02-06
### Fix
- increase max target length


<a name="v0.19.5"></a>
## [v0.19.5] - 2024-02-05
### Fix
- check targets must be valid label values


<a name="v0.19.4"></a>
## [v0.19.4] - 2024-01-30
### Fix
- allow scripted checks to have anything as the target value ([#592](https://github.com/grafana/synthetic-monitoring-agent/issues/592))


<a name="v0.19.3"></a>
## [v0.19.3] - 2023-12-13
### Fix
- test release on PRs


<a name="v0.19.2"></a>
## [v0.19.2] - 2023-12-13

<a name="v0.19.1"></a>
## [v0.19.1] - 2023-11-20

<a name="v0.19.0"></a>
## [v0.19.0] - 2023-11-07
### Feature
- add k6 to docker image

### Fix
- make the k6 runner timeout configurable ([#554](https://github.com/grafana/synthetic-monitoring-agent/issues/554))
- add a `name` label to metrics
- add k6 binary to release files


<a name="v0.18.3"></a>
## [v0.18.3] - 2023-10-27
### Fix
- make sure the String() methods match the proto defintion


<a name="v0.18.2"></a>
## [v0.18.2] - 2023-10-25

<a name="v0.18.1"></a>
## [v0.18.1] - 2023-10-13

<a name="v0.18.0"></a>
## [v0.18.0] - 2023-10-12
### Feature
- add support for interpolating variables


<a name="v0.17.3"></a>
## [v0.17.3] - 2023-09-28

<a name="v0.17.2"></a>
## [v0.17.2] - 2023-09-27
### Fix
- handle failed counter correctly


<a name="v0.17.1"></a>
## [v0.17.1] - 2023-09-14
### Feature
- keep track of scraper executions on a per-tenant level

### Fix
- add type to failure metrics
- for CSS selectors, the expression is not a predicate
- remove --discard-response-bodies


<a name="v0.17.0"></a>
## [v0.17.0] - 2023-09-05
### Feature
- use expression to match specific headers in multiHTTP

### Fix
- use double quotes with JS-escaped strings
- headers object might have extra commas
- pass body to HTTP request if specified


<a name="v0.16.5"></a>
## [v0.16.5] - 2023-07-14
### Fix
- don't use 0 in subject and condition enums


<a name="v0.16.4"></a>
## [v0.16.4] - 2023-07-05

<a name="v0.16.3"></a>
## [v0.16.3] - 2023-06-13

<a name="v0.16.2"></a>
## [v0.16.2] - 2023-06-07

<a name="v0.16.1"></a>
## [v0.16.1] - 2023-06-07

<a name="v0.16.0"></a>
## [v0.16.0] - 2023-06-06
### Fix
- parametrize the k6 runner


<a name="v0.15.0"></a>
## [v0.15.0] - 2023-05-23
### Fix
- JSON path value assertion needs expression and value


<a name="v0.14.5"></a>
## [v0.14.5] - 2023-04-27
### Fix
- truncate long label values


<a name="v0.14.4"></a>
## [v0.14.4] - 2023-04-19
### Build
- Don't expose drone secrets on PR builds ([#431](https://github.com/grafana/synthetic-monitoring-agent/issues/431))

### Fix
- Use Go 1.20.3 to build Agent ([#430](https://github.com/grafana/synthetic-monitoring-agent/issues/430))


<a name="v0.14.3"></a>
## [v0.14.3] - 2023-03-09
### Fix
- use proxy values from environment in metrics publisher


<a name="v0.14.2"></a>
## [v0.14.2] - 2023-02-23
### Fix
- do not resolve target in http with proxy


<a name="v0.14.1"></a>
## [v0.14.1] - 2023-01-25
### Fix
- setup timeout in ad-hoc checks


<a name="v0.14.0"></a>
## [v0.14.0] - 2023-01-09
### Feature
- Support global IDs in checks and tenants ([#389](https://github.com/grafana/synthetic-monitoring-agent/issues/389))


<a name="v0.13.0"></a>
## [v0.13.0] - 2022-12-15
### Feature
- add support for proxy connect headers
- update BBE to version 0.23.0

### Fix
- remove uses of io/ioutil


<a name="v0.12.1"></a>
## [v0.12.1] - 2022-12-07

<a name="v0.12.0"></a>
## [v0.12.0] - 2022-11-30
### Adhoc
- Reorder validation of adhoc checks

### Fix
- default to listening on localhost, not all interfaces
- allow getting API token from environment


<a name="v0.11.2"></a>
## [v0.11.2] - 2022-11-24

<a name="v0.11.1"></a>
## [v0.11.1] - 2022-11-23
### Fix
- WANTED_OSES / WANTED_ARCHES was removed, use PLATFORMS
- update MTR package


<a name="v0.11.0"></a>
## [v0.11.0] - 2022-11-17
### Fix
- set up backoffer to adhoc handler ([#363](https://github.com/grafana/synthetic-monitoring-agent/issues/363))

### Grpc
- Reduce size of objects in memory ([#368](https://github.com/grafana/synthetic-monitoring-agent/issues/368))

### Revert
- handle connection state changes ([#366](https://github.com/grafana/synthetic-monitoring-agent/issues/366))


<a name="v0.10.2"></a>
## [v0.10.2] - 2022-11-03
### Fix
- update .gitignore pattern


<a name="v0.10.1"></a>
## [v0.10.1] - 2022-11-03

<a name="v0.10.0"></a>
## [v0.10.0] - 2022-11-03
### Build
- prevent invalid os/arch combinations ([#336](https://github.com/grafana/synthetic-monitoring-agent/issues/336))

### Fix
- handle connection state changes

### Grpc
- Send known checks to API on connect ([#351](https://github.com/grafana/synthetic-monitoring-agent/issues/351))


<a name="v0.9.4"></a>
## [v0.9.4] - 2022-08-23
### Fix
- relax DNS target validation
- reject passwords in HTTP urls


<a name="v0.9.3"></a>
## [v0.9.3] - 2022-06-14

<a name="v0.9.2"></a>
## [v0.9.2] - 2022-06-13
### Fix
- correctly propagate conectivity errors
- enable HTTP2 by default


<a name="v0.9.1"></a>
## [v0.9.1] - 2022-06-02
### Reverts
- Bump github.com/prometheus/common from 0.32.1 to 0.34.0


<a name="v0.9.0"></a>
## [v0.9.0] - 2022-06-02
### Feature
- publish .deb and .rpm packages for arm and arm64
- cross-compile binaries for ARM and ARM64
- add a connection health ping


<a name="v0.8.2"></a>
## [v0.8.2] - 2022-04-26
### Feat
- Add a metric for failure to publish data ([#280](https://github.com/grafana/synthetic-monitoring-agent/issues/280))

### Fix
- fix http status code parsing for publish ([#279](https://github.com/grafana/synthetic-monitoring-agent/issues/279))


<a name="v0.8.1"></a>
## [v0.8.1] - 2022-03-29
### Fix
- update DNS tests to account for updated Recursion field
- Re-enable request recursion


<a name="v0.8.0"></a>
## [v0.8.0] - 2022-03-22
### Feature
- Ad-hoc checks


<a name="v0.7.1"></a>
## [v0.7.1] - 2022-03-14
### Fix
- 401 handling seems to be wrong


<a name="v0.7.0"></a>
## [v0.7.0] - 2022-03-03
### Feature
- Implement alternative ICMP prober


<a name="v0.6.3"></a>
## [v0.6.3] - 2022-03-01
### Fix
- DNS checks are passing the wrong target value to BBE


<a name="v0.6.2"></a>
## [v0.6.2] - 2022-01-28

<a name="v0.6.1"></a>
## [v0.6.1] - 2022-01-28

<a name="v0.6.0"></a>
## [v0.6.0] - 2022-01-27
### Chore
- Cleanup old circleci config ([#255](https://github.com/grafana/synthetic-monitoring-agent/issues/255))

### Feature
- add /disconnect endpoint
- trigger argo workflows on release ([#256](https://github.com/grafana/synthetic-monitoring-agent/issues/256))


<a name="v0.5.0"></a>
## [v0.5.0] - 2022-01-20
### Feature
- increase maximum number of user labels


<a name="v0.4.1"></a>
## [v0.4.1] - 2021-12-02
### Fix
- Add a exponential backoff to reconnections
- correctly propagate check failure


<a name="v0.4.0"></a>
## [v0.4.0] - 2021-11-30
### Feature
- add /ready endpoint for readiness probe
- enable traceroute checks by default ([#241](https://github.com/grafana/synthetic-monitoring-agent/issues/241))
- add log labels to log entries ([#240](https://github.com/grafana/synthetic-monitoring-agent/issues/240))


<a name="v0.3.3"></a>
## [v0.3.3] - 2021-11-16
### Fix
- errorCounter needs three labels


<a name="v0.3.2"></a>
## [v0.3.2] - 2021-11-04

<a name="v0.3.1"></a>
## [v0.3.1] - 2021-11-04

<a name="v0.3.0"></a>
## [v0.3.0] - 2021-10-26
### Feature
- add deprecated flag to probes ([#236](https://github.com/grafana/synthetic-monitoring-agent/issues/236))


<a name="v0.2.0"></a>
## [v0.2.0] - 2021-09-30
### Feature
- disconnect agent from API on signal
- report API connection status


<a name="v0.1.5"></a>
## [v0.1.5] - 2021-09-15
### Fix
- remove direct dependency on github.com/grafana/loki


<a name="v0.1.4"></a>
## [v0.1.4] - 2021-08-31
### Fix
- update fpm to 1.13.1


<a name="v0.1.3"></a>
## [v0.1.3] - 2021-08-30

<a name="v0.1.2"></a>
## [v0.1.2] - 2021-08-26
### Fix
- check if the incoming check is a traceroute one


<a name="v0.1.1"></a>
## [v0.1.1] - 2021-08-26

<a name="v0.1.0"></a>
## [v0.1.0] - 2021-08-25

<a name="v0.0.26"></a>
## [v0.0.26] - 2021-08-04

<a name="v0.0.25"></a>
## [v0.0.25] - 2021-08-03
### Feature
- report program's version

### Fix
- add +Inf bucket to histograms


<a name="v0.0.24"></a>
## [v0.0.24] - 2021-06-30

<a name="v0.0.23"></a>
## [v0.0.23] - 2021-06-21
### Feature
- add release script
- add support for publishing RPM packages
- add a features flag on the command line
- report overall test coverage

### Fix
- sign rpm packages and repo metadata
- Debian has createrepo, not createrepo-c


<a name="v0.0.22"></a>
## [v0.0.22] - 2021-05-10

<a name="v0.0.21"></a>
## [v0.0.21] - 2021-05-10

<a name="v0.0.20"></a>
## [v0.0.20] - 2021-04-28
### Feature
- validate HTTP headers

### Fix
- Add extra header validation tests


<a name="v0.0.19"></a>
## [v0.0.19] - 2021-03-30
### Change
- Increase the maximum label length to 128

### Fix
- check that there are no duplicate label names


<a name="v0.0.18"></a>
## [v0.0.18] - 2021-03-04
### Feature
- provide access to accounting map
- provide number of active series per check type
- add method to report check type

### Fix
- provide check type along with class info


<a name="v0.0.17"></a>
## [v0.0.17] - 2021-02-19
### Fix
- typo in client certificate and key


<a name="v0.0.16"></a>
## [v0.0.16] - 2021-01-29

<a name="v0.0.15"></a>
## [v0.0.15] - 2021-01-29

<a name="v0.0.14"></a>
## [v0.0.14] - 2021-01-07
### Feature
- add option to reduce the number of published metrics


<a name="v0.0.13"></a>
## [v0.0.13] - 2020-11-26
### Fix
- validate check and probe labels


<a name="v0.0.12"></a>
## [v0.0.12] - 2020-11-18

<a name="v0.0.11"></a>
## [v0.0.11] - 2020-11-11

<a name="v0.0.10"></a>
## [v0.0.10] - 2020-10-21
### Feature
- Add version, commit and buildstamp to Probe


<a name="v0.0.9"></a>
## [v0.0.9] - 2020-10-14
### Fix
- keep registering summaries and histograms


<a name="v0.0.8"></a>
## [v0.0.8] - 2020-10-14
### Fix
- be more flexible with what we accept for a FQHN


<a name="v0.0.7"></a>
## [v0.0.7] - 2020-09-25
### Build
- Add git-chglog configuration files

### Docs
- update and add links ([#78](https://github.com/grafana/synthetic-monitoring-agent/issues/78))

### Feature
- Implement test to check metric changes
- report probe version to API


<a name="v0.0.6"></a>
## [v0.0.6] - 2020-09-10
### Build
- update lint and test tools


<a name="v0.0.5"></a>
## [v0.0.5] - 2020-08-31
### Fix
- update blackbox_exporter to daa62bf75457


<a name="v0.0.4"></a>
## [v0.0.4] - 2020-08-26
### Build
- get version using scripts/version
- Fetch git tags in CircleCI


<a name="v0.0.3"></a>
## [v0.0.3] - 2020-08-26

<a name="v0.0.2"></a>
## [v0.0.2] - 2020-07-15

<a name="v0.0.1"></a>
## v0.0.1 - 2020-06-24

[Unreleased]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.28.0...HEAD
[v0.28.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.27.0...v0.28.0
[v0.27.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.26.0...v0.27.0
[v0.26.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.25.2...v0.26.0
[v0.25.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.25.1...v0.25.2
[v0.25.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.25.0...v0.25.1
[v0.25.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.3...v0.25.0
[v0.24.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.2...v0.24.3
[v0.24.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.1...v0.24.2
[v0.24.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.0...v0.24.1
[v0.24.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.4...v0.24.0
[v0.23.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.3...v0.23.4
[v0.23.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.2...v0.23.3
[v0.23.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.1...v0.23.2
[v0.23.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.0...v0.23.1
[v0.23.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.22.0...v0.23.0
[v0.22.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.21.0...v0.22.0
[v0.21.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.20.1...v0.21.0
[v0.20.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.6...v0.20.1
[v0.19.6]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.5...v0.19.6
[v0.19.5]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.4...v0.19.5
[v0.19.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.3...v0.19.4
[v0.19.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.2...v0.19.3
[v0.19.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.1...v0.19.2
[v0.19.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.0...v0.19.1
[v0.19.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.18.3...v0.19.0
[v0.18.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.18.2...v0.18.3
[v0.18.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.18.1...v0.18.2
[v0.18.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.18.0...v0.18.1
[v0.18.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.17.3...v0.18.0
[v0.17.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.17.2...v0.17.3
[v0.17.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.17.1...v0.17.2
[v0.17.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.17.0...v0.17.1
[v0.17.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.5...v0.17.0
[v0.16.5]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.4...v0.16.5
[v0.16.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.3...v0.16.4
[v0.16.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.2...v0.16.3
[v0.16.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.1...v0.16.2
[v0.16.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.16.0...v0.16.1
[v0.16.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.15.0...v0.16.0
[v0.15.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.5...v0.15.0
[v0.14.5]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.4...v0.14.5
[v0.14.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.3...v0.14.4
[v0.14.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.2...v0.14.3
[v0.14.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.1...v0.14.2
[v0.14.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.14.0...v0.14.1
[v0.14.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.13.0...v0.14.0
[v0.13.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.12.1...v0.13.0
[v0.12.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.12.0...v0.12.1
[v0.12.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.11.2...v0.12.0
[v0.11.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.11.1...v0.11.2
[v0.11.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.11.0...v0.11.1
[v0.11.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.10.2...v0.11.0
[v0.10.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.10.1...v0.10.2
[v0.10.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.10.0...v0.10.1
[v0.10.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.9.4...v0.10.0
[v0.9.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.9.3...v0.9.4
[v0.9.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.9.2...v0.9.3
[v0.9.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.9.1...v0.9.2
[v0.9.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.9.0...v0.9.1
[v0.9.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.8.2...v0.9.0
[v0.8.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.8.1...v0.8.2
[v0.8.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.8.0...v0.8.1
[v0.8.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.7.1...v0.8.0
[v0.7.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.7.0...v0.7.1
[v0.7.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.6.3...v0.7.0
[v0.6.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.6.2...v0.6.3
[v0.6.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.4.1...v0.5.0
[v0.4.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.3.3...v0.4.0
[v0.3.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.5...v0.2.0
[v0.1.5]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.4...v0.1.5
[v0.1.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.26...v0.1.0
[v0.0.26]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.25...v0.0.26
[v0.0.25]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.24...v0.0.25
[v0.0.24]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.23...v0.0.24
[v0.0.23]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.22...v0.0.23
[v0.0.22]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.21...v0.0.22
[v0.0.21]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.20...v0.0.21
[v0.0.20]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.19...v0.0.20
[v0.0.19]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.16...v0.0.17
[v0.0.16]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.11...v0.0.12
[v0.0.11]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.10...v0.0.11
[v0.0.10]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.0.1...v0.0.2
