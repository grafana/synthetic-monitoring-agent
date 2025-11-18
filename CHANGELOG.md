# Changelog

## [0.47.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.46.1...v0.47.0) (2025-11-17)


### Features

* make `probe_duration_seconds` match `probe_script_duration_seconds` ([eb7901c](https://github.com/grafana/synthetic-monitoring-agent/commit/eb7901c3e4ccb6785c0958254df32308b5816bed))


### Fixes

* Remove old +build directives ([22a3d7d](https://github.com/grafana/synthetic-monitoring-agent/commit/22a3d7d1d2d70b23bd3aeac656e4d537985395b2))


### Miscellaneous Chores

* Update actions/checkout action to v5.0.1 ([5c90982](https://github.com/grafana/synthetic-monitoring-agent/commit/5c90982e5db9fbd284c669a1b32c6d21996504a9))
* Update actions/checkout digest to 93cb6ef ([de3dc32](https://github.com/grafana/synthetic-monitoring-agent/commit/de3dc327cdd16ca9fd5c93d7b04666d50b27861b))
* Update dependency grafana/xk6-sm to v0.6.11 ([171d090](https://github.com/grafana/synthetic-monitoring-agent/commit/171d09024c7eff61baa0461e79ee00f879523b4d))
* Update docker:dind Docker digest to ecac43e ([e7b2533](https://github.com/grafana/synthetic-monitoring-agent/commit/e7b25332dffe48c084d9bdbc4865ada83d02a830))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.23.0 ([15e62bf](https://github.com/grafana/synthetic-monitoring-agent/commit/15e62bf58daf655169fc8e738b426d917a8051ea))
* Update golang.org/x/exp digest to e25ba8c ([13e910c](https://github.com/grafana/synthetic-monitoring-agent/commit/13e910c1559df03787405b194405340ab3a1070d))
* Update grafana/shared-workflows digest to 92b146a ([28b78c4](https://github.com/grafana/synthetic-monitoring-agent/commit/28b78c4d413d1aa5628e75e917f6b062e970523f))
* Update module golang.org/x/net to v0.47.0 ([d782320](https://github.com/grafana/synthetic-monitoring-agent/commit/d782320a0984ee22db2bf95e37cfdf95a8eaa672))
* use the obviously superior way to `Unseconds()` ([dae01e8](https://github.com/grafana/synthetic-monitoring-agent/commit/dae01e8d18595880ac288ba47ff820718ddf1c62))

## [0.46.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.46.0...v0.46.1) (2025-11-12)


### Fixes

* Handle 504 Gateway Timeout false alarms in adhoc checks ([#1578](https://github.com/grafana/synthetic-monitoring-agent/issues/1578)) ([6a9302d](https://github.com/grafana/synthetic-monitoring-agent/commit/6a9302d7490af22b512c4c8515d47a3aefe36021))
* Update DEVELOPMENT.md ([#1600](https://github.com/grafana/synthetic-monitoring-agent/issues/1600)) ([d99b4db](https://github.com/grafana/synthetic-monitoring-agent/commit/d99b4db283ba443fbe016b8497fa959e431ae14b))
* Use ErrorIs instead of string comparison for context deadline ([#1597](https://github.com/grafana/synthetic-monitoring-agent/issues/1597)) ([2992b76](https://github.com/grafana/synthetic-monitoring-agent/commit/2992b7633fed6966284c97b74c859c3580e8b93f))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.6.10 ([c11ff5b](https://github.com/grafana/synthetic-monitoring-agent/commit/c11ff5bb152b1dc114f517e81e2eece2be60b14b))
* Update docker:dind Docker digest to 2a232a4 ([a90a0f3](https://github.com/grafana/synthetic-monitoring-agent/commit/a90a0f390482da1ac77ddaa4316af31988cba263))
* Update docker:dind Docker digest to 96789d5 ([#1599](https://github.com/grafana/synthetic-monitoring-agent/issues/1599)) ([fd12617](https://github.com/grafana/synthetic-monitoring-agent/commit/fd126176da5b2fbe6c217f77ca8872bc857db7cc))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v142 ([#1588](https://github.com/grafana/synthetic-monitoring-agent/issues/1588)) ([4e008d8](https://github.com/grafana/synthetic-monitoring-agent/commit/4e008d8f7d059b48bcbeb3036b53c384a4bdf523))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:142.0.7444.59-r0-3.22.2 Docker digest to 5dcd595 ([#1598](https://github.com/grafana/synthetic-monitoring-agent/issues/1598)) ([f537d70](https://github.com/grafana/synthetic-monitoring-agent/commit/f537d70d7ebe69ea2d0f2ba39ea695e65feebaa9))
* Update grafana/shared-workflows digest to 80f9be5 ([a62efa5](https://github.com/grafana/synthetic-monitoring-agent/commit/a62efa5ef6caf68f0882deb580ab75ed954a9477))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.19.0 ([#1589](https://github.com/grafana/synthetic-monitoring-agent/issues/1589)) ([0628cc6](https://github.com/grafana/synthetic-monitoring-agent/commit/0628cc6a43fbc7ac6f7ee8b03383730f3d74d977))
* Update module golang.org/x/sync to v0.18.0 ([0abb7b0](https://github.com/grafana/synthetic-monitoring-agent/commit/0abb7b0107845ce0517fc19c53ceae2ba40bedbd))

## [0.46.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.45.0...v0.46.0) (2025-11-03)


### Features

* add support for secrets in http checks ([#1469](https://github.com/grafana/synthetic-monitoring-agent/issues/1469)) ([61da457](https://github.com/grafana/synthetic-monitoring-agent/commit/61da45730b8a6e0a99b435c76d80d507338a96cc))
* Apply jitter to tenants cache ([#1576](https://github.com/grafana/synthetic-monitoring-agent/issues/1576)) ([35e4ebe](https://github.com/grafana/synthetic-monitoring-agent/commit/35e4ebe1643ebdd0957ed72feb65f00b730f852b))


### Fixes

* k6runner: properly propagate user errors back to the prober ([f58ee35](https://github.com/grafana/synthetic-monitoring-agent/commit/f58ee358b876235e37db47fdfd8df1324ef4ee08))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.6.9 ([#1586](https://github.com/grafana/synthetic-monitoring-agent/issues/1586)) ([d97c427](https://github.com/grafana/synthetic-monitoring-agent/commit/d97c42799e2ccfe3de05d2b36fc62c653a96e25e))
* Update docker:dind Docker digest to ea9d204 ([#1585](https://github.com/grafana/synthetic-monitoring-agent/issues/1585)) ([a5018c1](https://github.com/grafana/synthetic-monitoring-agent/commit/a5018c1ab77e3b0bf4bec5a927d9fa34c3e3c55a))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v141.0.7390.122-r0-3.22.2 ([#1570](https://github.com/grafana/synthetic-monitoring-agent/issues/1570)) ([ddd9e72](https://github.com/grafana/synthetic-monitoring-agent/commit/ddd9e7265045bb8c56734d77e9b7257147c5f074))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:141.0.7390.122-r0-3.22.2 Docker digest to a6d31b0 ([78a869e](https://github.com/grafana/synthetic-monitoring-agent/commit/78a869e3b197e435b901f178e255dd7d7270e9cc))
* Update GitHub Artifact Actions ([#1574](https://github.com/grafana/synthetic-monitoring-agent/issues/1574)) ([38f3dee](https://github.com/grafana/synthetic-monitoring-agent/commit/38f3dee543c5f5e5b0c3a7e985c998abaf33faa7))
* Update golang.org/x/exp digest to a4bb9ff ([#1572](https://github.com/grafana/synthetic-monitoring-agent/issues/1572)) ([7e978a4](https://github.com/grafana/synthetic-monitoring-agent/commit/7e978a439cf0ad55dbbfc771066447dcc99ba191))
* Update googleapis/release-please-action action to v4.4.0 ([#1571](https://github.com/grafana/synthetic-monitoring-agent/issues/1571)) ([66001d6](https://github.com/grafana/synthetic-monitoring-agent/commit/66001d6879605b9dc0ee8e4c1a70aaf398a88727))
* Update grafana/shared-workflows digest to ae92934 ([#1587](https://github.com/grafana/synthetic-monitoring-agent/issues/1587)) ([122997a](https://github.com/grafana/synthetic-monitoring-agent/commit/122997aae5cfa9f102824650577a5842b96c2b8e))
* Update module github.com/grafana/gsm-api-go-client to v0.2.1 ([#1581](https://github.com/grafana/synthetic-monitoring-agent/issues/1581)) ([58def6b](https://github.com/grafana/synthetic-monitoring-agent/commit/58def6b6d3725793d7f19eef8ee0775954798e1e))
* Update module kernel.org/pub/linux/libs/security/libcap/cap to v1.2.77 ([#1582](https://github.com/grafana/synthetic-monitoring-agent/issues/1582)) ([496aa33](https://github.com/grafana/synthetic-monitoring-agent/commit/496aa337ef6d769477590e877ed11e707a7146a9))

## [0.45.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.44.2...v0.45.0) (2025-10-29)


### Features

* Adjust publisher options ([#1558](https://github.com/grafana/synthetic-monitoring-agent/issues/1558)) ([b782a3c](https://github.com/grafana/synthetic-monitoring-agent/commit/b782a3c08117d1bf07961847c36095348d5028ac))


### Fixes

* Run modernize over source code ([8e7fee2](https://github.com/grafana/synthetic-monitoring-agent/commit/8e7fee2f5383df5c3666120fbc6351f718a008eb))
* Update some github.com/prometheus/* modules ([67340ec](https://github.com/grafana/synthetic-monitoring-agent/commit/67340ec0f334c30768e0c2b42e222173bdd0b932))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.6.8 ([#1566](https://github.com/grafana/synthetic-monitoring-agent/issues/1566)) ([2ca5380](https://github.com/grafana/synthetic-monitoring-agent/commit/2ca5380f6c06354256c8fb6286ef969c75e2bd21))
* Update grafana/shared-workflows digest to c8ac4a2 ([#1573](https://github.com/grafana/synthetic-monitoring-agent/issues/1573)) ([14150a5](https://github.com/grafana/synthetic-monitoring-agent/commit/14150a56737ac3b93b509ae8e4574584e788afee))
* Update grafana/shared-workflows digest to eed6978 ([#1567](https://github.com/grafana/synthetic-monitoring-agent/issues/1567)) ([082535c](https://github.com/grafana/synthetic-monitoring-agent/commit/082535ccf3a9f93bed55e348392f9e79cb49059e))
* Update module github.com/KimMachineGun/automemlimit to v0.7.5 ([9dcd051](https://github.com/grafana/synthetic-monitoring-agent/commit/9dcd051df110152d518497515cf7ad931b9aa527))

## [0.44.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.44.1...v0.44.2) (2025-10-15)


### Miscellaneous Chores

* Update alpine Docker tag to v3.22.2 ([8f637a5](https://github.com/grafana/synthetic-monitoring-agent/commit/8f637a53f2bf5dc829f406866520bfe0117bee1b))
* Update dependency grafana/xk6-sm to v0.6.7 ([78e82fc](https://github.com/grafana/synthetic-monitoring-agent/commit/78e82fca503c03514c19cfd3ff6fb63745700342))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v141.0.7390.76-r0-3.22.2 ([bd6a7c4](https://github.com/grafana/synthetic-monitoring-agent/commit/bd6a7c475b6d88fdbbc98f95ca7fc252aa769fd2))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.21.3 ([a57e2a4](https://github.com/grafana/synthetic-monitoring-agent/commit/a57e2a4d7b86b28881085eb1ec087a9759adf4d5))
* Update golang.org/x/exp digest to d2f985d ([e4d957b](https://github.com/grafana/synthetic-monitoring-agent/commit/e4d957bbaa2f0d1f4bd062a0587f2f512ee1af77))
* Update module golang.org/x/net to v0.46.0 ([73c4182](https://github.com/grafana/synthetic-monitoring-agent/commit/73c4182c2fb5e1ceaad260b54840b25e084dfd5d))
* Update module google.golang.org/grpc to v1.76.0 ([469d75b](https://github.com/grafana/synthetic-monitoring-agent/commit/469d75bbbd67c532792e3e1702b7df38a2f0c242))

## [0.44.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.44.0...v0.44.1) (2025-10-14)


### Fixes

* Revert "fix: Short circuit get tenant request ([#1556](https://github.com/grafana/synthetic-monitoring-agent/issues/1556))" ([#1560](https://github.com/grafana/synthetic-monitoring-agent/issues/1560)) ([1d998b1](https://github.com/grafana/synthetic-monitoring-agent/commit/1d998b1fdeb4abab8901d87ca1b451bc634ceecc))

## [0.44.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.43.3...v0.44.0) (2025-10-13)


### Features

* Add cost attribution labels to tenants ([#1543](https://github.com/grafana/synthetic-monitoring-agent/issues/1543)) ([cf8f746](https://github.com/grafana/synthetic-monitoring-agent/commit/cf8f74603c188f61900b01d27d8b7c857ec856d5))


### Fixes

* don't cache tenants if the secret store is not configured ([36ea8a7](https://github.com/grafana/synthetic-monitoring-agent/commit/36ea8a79f4a4f7e0ae47f1ca82c42df30b0a2fce))
* handle nil secret store in calculateValidUntil ([f19ed84](https://github.com/grafana/synthetic-monitoring-agent/commit/f19ed84e04a76348598a1a75fc7d7b81139f56d9))
* Short circuit get tenant request ([#1556](https://github.com/grafana/synthetic-monitoring-agent/issues/1556)) ([f5634c0](https://github.com/grafana/synthetic-monitoring-agent/commit/f5634c0ee60a64a3df5609d234a68c85f427d728))


### Miscellaneous Chores

* Update docker:dind Docker digest to 2417311 ([a8b3ba3](https://github.com/grafana/synthetic-monitoring-agent/commit/a8b3ba3ce6b9f6ea2cb0e79baa9ba1207ceac347))
* Update docker:dind Docker digest to c090c93 ([c5e71f4](https://github.com/grafana/synthetic-monitoring-agent/commit/c5e71f49d36f6aec7469396ac50b012b8120e481))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v141 ([874fdfd](https://github.com/grafana/synthetic-monitoring-agent/commit/874fdfda02f1fe56b550018f1489394e7d0a9ddd))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:140.0.7339.207-r0-3.22.1 Docker digest to 44e30e6 ([af8e47e](https://github.com/grafana/synthetic-monitoring-agent/commit/af8e47e1e24ffb7a41d91d0c71937e890e10471a))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.21.1 ([f2521c9](https://github.com/grafana/synthetic-monitoring-agent/commit/f2521c9e12a68d19b5149677ec11f175fb425f2f))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.21.2 ([a0c4dfe](https://github.com/grafana/synthetic-monitoring-agent/commit/a0c4dfec85a414f22c715c6a7e123dc03c0f8088))
* Update golang.org/x/exp digest to 27f1f14 ([3639836](https://github.com/grafana/synthetic-monitoring-agent/commit/36398363acfa14b21e539164cc74bd5c7628a9cc))
* Update grafana/shared-workflows digest to f248984 ([98dd64c](https://github.com/grafana/synthetic-monitoring-agent/commit/98dd64cd7e64aadf56e4711457f496c7641d2ba8))
* Update module github.com/go-logfmt/logfmt to v0.6.1 ([37c1f35](https://github.com/grafana/synthetic-monitoring-agent/commit/37c1f3565b0e6b806393c7a979998d9819bfed6d))

## [0.43.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.43.2...v0.43.3) (2025-10-06)


### Fixes

* add warning when tenant received without secret store details ([#1533](https://github.com/grafana/synthetic-monitoring-agent/issues/1533)) ([708100b](https://github.com/grafana/synthetic-monitoring-agent/commit/708100b81fd0e0b861260a2dcceebb2d2fa37e6f))


### Miscellaneous Chores

* add further debug logging around secrets ([#1542](https://github.com/grafana/synthetic-monitoring-agent/issues/1542)) ([ad27c2a](https://github.com/grafana/synthetic-monitoring-agent/commit/ad27c2aca7759ad14d7107364f856f26a060f437))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v140.0.7339.207-r0-3.22.1 ([#1532](https://github.com/grafana/synthetic-monitoring-agent/issues/1532)) ([344d0de](https://github.com/grafana/synthetic-monitoring-agent/commit/344d0de713b75c272f73193a7b9c1a736e0730ce))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:140.0.7339.207-r0-3.22.1 Docker digest to 109f5c2 ([#1535](https://github.com/grafana/synthetic-monitoring-agent/issues/1535)) ([0d5fe00](https://github.com/grafana/synthetic-monitoring-agent/commit/0d5fe000a2158681abde2816af1901d53c310a40))

## [0.43.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.43.1...v0.43.2) (2025-09-29)


### Fixes

* Change default logging level to WarnLevel ([#1525](https://github.com/grafana/synthetic-monitoring-agent/issues/1525)) ([d21a1aa](https://github.com/grafana/synthetic-monitoring-agent/commit/d21a1aa88a1c62e8c0726551e3b07823819b8dd0))
* Do not block during probe registration ([f691bdb](https://github.com/grafana/synthetic-monitoring-agent/commit/f691bdb03c0e809d98b8c86af1b78ccf9e30012f))


### Miscellaneous Chores

* improve wording about how to disable default blocked networks ([19c7f3f](https://github.com/grafana/synthetic-monitoring-agent/commit/19c7f3fceeb0fed1b951b172d83e30278d3f0f9e))
* Update actions/cache action to v4.3.0 ([1ac7f7a](https://github.com/grafana/synthetic-monitoring-agent/commit/1ac7f7a1651b6e467d1d279648de6fd267a040e1))
* Update dependency grafana/xk6-sm to v0.6.6 ([a0d1e89](https://github.com/grafana/synthetic-monitoring-agent/commit/a0d1e89aebd61fee4e0db76a536d5435010f7cd0))
* Update docker:dind Docker digest to 2ceb471 ([1e0bcd9](https://github.com/grafana/synthetic-monitoring-agent/commit/1e0bcd9380f6dc06c9ca4cafe39dbf0bdeba7351))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v140 ([1815e22](https://github.com/grafana/synthetic-monitoring-agent/commit/1815e227d475ff6b05b13580b054b20f5f6a8fab))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:140.0.7339.185-r0-3.22.1 Docker digest to 130fb63 ([0de3468](https://github.com/grafana/synthetic-monitoring-agent/commit/0de34683a6edc4b239d880af358bcdc8fdd2eade))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.21.0 ([78ad18f](https://github.com/grafana/synthetic-monitoring-agent/commit/78ad18f389dea40b583f1bb05422826310b9e753))
* Update grafana/shared-workflows digest to 052ae53 ([a7428ff](https://github.com/grafana/synthetic-monitoring-agent/commit/a7428ff1f8d6db81102050e4fe7cc51a842fe132))
* Update grafana/shared-workflows digest to bfeac15 ([aaa8c04](https://github.com/grafana/synthetic-monitoring-agent/commit/aaa8c0445d619cbbc10235e1dfb6ea62b23b7093))
* Update module github.com/quasilyte/go-ruleguard/dsl to v0.3.23 ([ae78cbd](https://github.com/grafana/synthetic-monitoring-agent/commit/ae78cbdd2bc7dc9f8b4b8c5c086e63ea2941f968))

## [0.43.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.43.0...v0.43.1) (2025-09-23)


### Fixes

* add a warning when secret store configuration is missing ([#1519](https://github.com/grafana/synthetic-monitoring-agent/issues/1519)) ([6a7c662](https://github.com/grafana/synthetic-monitoring-agent/commit/6a7c662b573e13f9337a7f57a979ed13076c4917))
* Modernize the agent ([#1503](https://github.com/grafana/synthetic-monitoring-agent/issues/1503)) ([fa27e0e](https://github.com/grafana/synthetic-monitoring-agent/commit/fa27e0ed99d837da612762c53f942229aca0d9f1))


### Miscellaneous Chores

* Update actions/create-github-app-token action to v2.1.4 ([f91c3ee](https://github.com/grafana/synthetic-monitoring-agent/commit/f91c3ee92401d4e926170d1484b2471b6e3971f2))
* Update dependency grafana/xk6-sm to v0.6.5 ([2854c82](https://github.com/grafana/synthetic-monitoring-agent/commit/2854c8216ceb4e22ac89708414f322c95f37cde4))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:139.0.7258.154-r0-3.22.1 Docker digest to 8a396e1 ([8f40016](https://github.com/grafana/synthetic-monitoring-agent/commit/8f400166765dae4b87c3f9c1bdd3fd5fa61e0496))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.20.0 ([d9e0998](https://github.com/grafana/synthetic-monitoring-agent/commit/d9e0998dc16e042c5d82b8ec1b9a9aefab35755d))
* Update golang.org/x/exp digest to df92998 ([2316bf4](https://github.com/grafana/synthetic-monitoring-agent/commit/2316bf446ff42ee43d504a516786f46f9f08f4f6))
* Update grafana/shared-workflows digest to c194c0f ([bd5b343](https://github.com/grafana/synthetic-monitoring-agent/commit/bd5b34388e08a233578e9d03ec503fd043251d88))

## [0.43.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.42.0...v0.43.0) (2025-09-15)


### Features

* Add air configuration ([#1489](https://github.com/grafana/synthetic-monitoring-agent/issues/1489)) ([acbe385](https://github.com/grafana/synthetic-monitoring-agent/commit/acbe385f0f61b90fb3ba1fb2c478085dbfd220bd))
* Add PATCH as a valid HTTP method ([cbcbc16](https://github.com/grafana/synthetic-monitoring-agent/commit/cbcbc16dc89b6f856727e3c751619bf1f8b5fe2e))


### Fixes

* Fail earlier if sm-k6 isn't on path ([#1488](https://github.com/grafana/synthetic-monitoring-agent/issues/1488)) ([eb46b7d](https://github.com/grafana/synthetic-monitoring-agent/commit/eb46b7dcf1ca781c58ddfdc6afcb5934c939c8ea))
* make log `duration_seconds` the same as the namesake metric ([96a7682](https://github.com/grafana/synthetic-monitoring-agent/commit/96a7682dd916e5bd27727134e0f957004a4cd36f))
* Update active series estimates ([ad83d38](https://github.com/grafana/synthetic-monitoring-agent/commit/ad83d382e8cac6cfed07fedbd8e50e413b458722))


### Miscellaneous Chores

* k6/http: log checkInfo when when retries are exhausted ([9f88089](https://github.com/grafana/synthetic-monitoring-agent/commit/9f880896c785d7132f5abf19afb56fabb1ff3279))
* Update actions/setup-go action to v6 ([dc52803](https://github.com/grafana/synthetic-monitoring-agent/commit/dc5280392d2e439a486e075d3219294d5e482ca3))
* Update dependency grafana/xk6-sm to v0.6.4 ([ddb1588](https://github.com/grafana/synthetic-monitoring-agent/commit/ddb15889bd9212e3c7fe28bc250b4f22a62184de))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:139.0.7258.154-r0-3.22.1 Docker digest to 59612ba ([f1c47ce](https://github.com/grafana/synthetic-monitoring-agent/commit/f1c47ce25a6105687f88d634dd05ace87e6c6f27))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:139.0.7258.154-r0-3.22.1 Docker digest to 6b2ac21 ([833ed3a](https://github.com/grafana/synthetic-monitoring-agent/commit/833ed3a974382a7cf3df2f6ba65682a702f4b507))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.19.0 ([8d14313](https://github.com/grafana/synthetic-monitoring-agent/commit/8d14313f7cc9aed477e70a110aeaf0b9c3033104))
* update grafana.com/loki/pkg/push ([c2c60af](https://github.com/grafana/synthetic-monitoring-agent/commit/c2c60af723e09bae1207d82490e179cd20cf70a7))
* Update module github.com/spf13/afero to v1.15.0 ([d6ee6eb](https://github.com/grafana/synthetic-monitoring-agent/commit/d6ee6eb371942a3bbc8dba7853487a1bbf058e15))
* Update module golang.org/x/net to v0.44.0 ([5d32e94](https://github.com/grafana/synthetic-monitoring-agent/commit/5d32e94cf128e0ee409d84ada97ec8d24332c645))
* Update module google.golang.org/grpc to v1.75.1 ([26fd4c0](https://github.com/grafana/synthetic-monitoring-agent/commit/26fd4c0da77cd5e476f0cee06e2622df469d6a13))

## [0.42.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.41.3...v0.42.0) (2025-09-08)


### Features

* lay groundwork for protocol secrets and string interpolation system ([#1468](https://github.com/grafana/synthetic-monitoring-agent/issues/1468)) ([8d634bb](https://github.com/grafana/synthetic-monitoring-agent/commit/8d634bb30f64a7eda9e367cdc8af7eb65c5ab8cd))


### Miscellaneous Chores

* k6runner/env: modularize set-env-if-empty code ([b1b8307](https://github.com/grafana/synthetic-monitoring-agent/commit/b1b8307cad5598ada4883d2e2ef5a7679ddd64b7))
* k6runner/env: set `K6_AUTO_EXTENSION_RESOLUTION=false` by default ([c237b99](https://github.com/grafana/synthetic-monitoring-agent/commit/c237b9969fdcde759be7a8e9c67364b6842377c5))
* switch to platform-managed renovate ([1ecfa1b](https://github.com/grafana/synthetic-monitoring-agent/commit/1ecfa1b0114d28084ba59f4d47750cfb559af3aa))
* Update dependency grafana/xk6-sm to v0.6.3 ([2f00786](https://github.com/grafana/synthetic-monitoring-agent/commit/2f007860179d9df8f9ed325b76fa8d80e6d9a7a6))
* Update docker:dind Docker digest to 8316442 ([90ab790](https://github.com/grafana/synthetic-monitoring-agent/commit/90ab79056c112fe8c165fa9528610156ab99bc9b))
* Update docker:dind Docker digest to a56b3bd ([5db203d](https://github.com/grafana/synthetic-monitoring-agent/commit/5db203da6864bcaace908bdc285cb1ad2134e82e))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.18.1 ([f5cd0e9](https://github.com/grafana/synthetic-monitoring-agent/commit/f5cd0e99b108ad52fcf16097cf134f6ad5f1e198))
* Update grafana/shared-workflows digest to ad19c58 ([#1491](https://github.com/grafana/synthetic-monitoring-agent/issues/1491)) ([8c93e0f](https://github.com/grafana/synthetic-monitoring-agent/commit/8c93e0fe31af09f497c81f54ea7346e1604c70a8))
* Update grafana/shared-workflows digest to bee55b8 ([c31b3ad](https://github.com/grafana/synthetic-monitoring-agent/commit/c31b3ada1b5b5bd9b19e855b6337de2d4835c236))
* Update grafana/shared-workflows digest to ea90509 ([#1492](https://github.com/grafana/synthetic-monitoring-agent/issues/1492)) ([5fbc679](https://github.com/grafana/synthetic-monitoring-agent/commit/5fbc679ba0465ce97b71987801f3946ff8a271b8))
* Update module golang.org/x/sync to v0.17.0 ([#1490](https://github.com/grafana/synthetic-monitoring-agent/issues/1490)) ([b44cf50](https://github.com/grafana/synthetic-monitoring-agent/commit/b44cf50644210aa24890595c3fb19f6fa8b4da74))

## [0.41.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.41.2...v0.41.3) (2025-08-30)


### Fixes

* extract testSecretStore helpers to testhelper package ([#1463](https://github.com/grafana/synthetic-monitoring-agent/issues/1463)) ([c21d763](https://github.com/grafana/synthetic-monitoring-agent/commit/c21d763fd435913bc20ae3d13d6fc57b3c40af9f))
* Make tests less flaky ([9f4e479](https://github.com/grafana/synthetic-monitoring-agent/commit/9f4e479ce46fb2e729ced943ce01febb321bb69d))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.6.2 ([12726d8](https://github.com/grafana/synthetic-monitoring-agent/commit/12726d84cc0d9e8288c426addc0fec6be25b7fab))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v139.0.7258.154-r0-3.22.1 ([9288f87](https://github.com/grafana/synthetic-monitoring-agent/commit/9288f875a3ff0dd9c7dad83c5d4fb6eb69c8ad92))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.17.0 ([f9de2c3](https://github.com/grafana/synthetic-monitoring-agent/commit/f9de2c3ad4f871ccb7b3edf73eb504f47302ef01))

## [0.41.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.41.1...v0.41.2) (2025-08-27)


### Fixes

* Set up DinD in build and publish workflow ([e20fd35](https://github.com/grafana/synthetic-monitoring-agent/commit/e20fd35556ac3155cc72eaf73305b705e7b68a17))

## [0.41.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.41.0...v0.41.1) (2025-08-27)


### Fixes

* Migrate golangci-lint to v2 ([ce7fbb2](https://github.com/grafana/synthetic-monitoring-agent/commit/ce7fbb27b6b10ca857c0afd0f0612c753febcfc0))
* properly classify codes.Unavailable errors as TransientError ([#1457](https://github.com/grafana/synthetic-monitoring-agent/issues/1457)) ([2435423](https://github.com/grafana/synthetic-monitoring-agent/commit/2435423d3e8e7c19baa55b4bedff94f41a14c72b))


### Miscellaneous Chores

* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.16.1 ([c13c0d7](https://github.com/grafana/synthetic-monitoring-agent/commit/c13c0d7a96558c793fc1bc7715a4f87f48cac94d))
* Update module github.com/stretchr/testify to v1.11.1 ([446cc2b](https://github.com/grafana/synthetic-monitoring-agent/commit/446cc2b1bbcf1d08ac31cb3caebcdb0709c380b8))

## [0.41.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.40.1...v0.41.0) (2025-08-26)


### Features

* runner/local: run k6 with `--throw` to raise common errors as exceptions ([f31a1d1](https://github.com/grafana/synthetic-monitoring-agent/commit/f31a1d1e5287bc8c086f69d01f521023fb15a20c))


### Fixes

* Remove the requirement for a prefix on secret strings ([#1446](https://github.com/grafana/synthetic-monitoring-agent/issues/1446)) ([f311bd1](https://github.com/grafana/synthetic-monitoring-agent/commit/f311bd10f5f99bde57f8f0405703e92601e0ed78))
* Workaround issues in build-push-to-dockerhub ([02ded1f](https://github.com/grafana/synthetic-monitoring-agent/commit/02ded1f6e030dc7483f8b6e894dfd1632a6443c5))


### Miscellaneous Chores

* Update actions/create-github-app-token action to v2.1.1 ([b9c3258](https://github.com/grafana/synthetic-monitoring-agent/commit/b9c32584e05b1d246457a09350a3173777302574))
* Update dependency grafana/xk6-sm to v0.6.1 ([223f443](https://github.com/grafana/synthetic-monitoring-agent/commit/223f443d8e059fd402818613a8c2760cad93f97a))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v139 ([78a2939](https://github.com/grafana/synthetic-monitoring-agent/commit/78a2939091d968d18545bea25d53e351e6ce6a70))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:139.0.7258.66-r0-3.22.1 Docker digest to 3e6e533 ([767abcf](https://github.com/grafana/synthetic-monitoring-agent/commit/767abcf28cc2eb06a36a1e0873379383fcc79127))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.14.0 ([f14a619](https://github.com/grafana/synthetic-monitoring-agent/commit/f14a6190f9c6160db4dcee33b49377094c38eb9b))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.15.0 ([bcc4cb4](https://github.com/grafana/synthetic-monitoring-agent/commit/bcc4cb4dbde8a13de535cdf6c2fd1ca8d71c24c3))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.16.0 ([#1451](https://github.com/grafana/synthetic-monitoring-agent/issues/1451)) ([2e7d83b](https://github.com/grafana/synthetic-monitoring-agent/commit/2e7d83b81b9cd289461b9de7ec16864007da53e7))
* Update golang.org/x/exp digest to 8b4c13b ([6b7d008](https://github.com/grafana/synthetic-monitoring-agent/commit/6b7d008a01d405d53a228408436ced162ca783bc))
* Update googleapis/release-please-action action to v4.3.0 ([5808b0d](https://github.com/grafana/synthetic-monitoring-agent/commit/5808b0dbeab1811edf9efdb20c27d7034605c98d))
* Update grafana/shared-workflows digest to 8bb65cb ([2de21f1](https://github.com/grafana/synthetic-monitoring-agent/commit/2de21f1d342097d3eedbfab27016c9071a2df726))
* Update module github.com/stretchr/testify to v1.11.0 ([#1449](https://github.com/grafana/synthetic-monitoring-agent/issues/1449)) ([6333526](https://github.com/grafana/synthetic-monitoring-agent/commit/6333526e8cfcb7ef5c1bcf9723a5b719436ff427))
* Update module google.golang.org/grpc to v1.75.0 ([326e626](https://github.com/grafana/synthetic-monitoring-agent/commit/326e626f9bf581853e31cc2f090d5881a4f11dec))

## [0.40.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.40.0...v0.40.1) (2025-08-15)


### Fixes

* Update the metrics labels limit const ([#1428](https://github.com/grafana/synthetic-monitoring-agent/issues/1428)) ([afa2f54](https://github.com/grafana/synthetic-monitoring-agent/commit/afa2f5487c517531f094ef51dbe650f25d62b30c))


### Miscellaneous Chores

* Update actions/checkout action to v5 ([#1424](https://github.com/grafana/synthetic-monitoring-agent/issues/1424)) ([ca42c9c](https://github.com/grafana/synthetic-monitoring-agent/commit/ca42c9c70a7910df14ab13bdd82dc6377ffd2964))
* Update dependency grafana/xk6-sm to v0.6.0 ([382724a](https://github.com/grafana/synthetic-monitoring-agent/commit/382724aa08d4f5121f27944cf8b577a35eafa7e0))

## [0.40.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.39.2...v0.40.0) (2025-08-11)


### Features

* add secret manager boolean to HTTP checks ([#1415](https://github.com/grafana/synthetic-monitoring-agent/issues/1415)) ([5c6b9a5](https://github.com/grafana/synthetic-monitoring-agent/commit/5c6b9a5a3c3fb65b74f4dfebb013d7f3c1fa8711))


### Fixes

* error out early if grpc address doesn't have a port ([b7fdb7f](https://github.com/grafana/synthetic-monitoring-agent/commit/b7fdb7fac7173e04eef8e03455f689649d079f9d))


### Miscellaneous Chores

* Update actions/cache action to v4.2.4 ([81ff4f1](https://github.com/grafana/synthetic-monitoring-agent/commit/81ff4f12b87bdd679416ecefe8299d2359e00ed7))
* Update actions/checkout digest to 08eba0b ([8222602](https://github.com/grafana/synthetic-monitoring-agent/commit/82226026d9f7480983754fc669e828ff4add4d1c))
* Update actions/create-github-app-token action to v2.1.0 ([#1419](https://github.com/grafana/synthetic-monitoring-agent/issues/1419)) ([e738a54](https://github.com/grafana/synthetic-monitoring-agent/commit/e738a542d5c6a488b14eb2c070fac0db07b3615f))
* Update actions/download-artifact action to v5 ([5096bcc](https://github.com/grafana/synthetic-monitoring-agent/commit/5096bcc09efdf1b92da6a8737421df8a2de1fc80))
* Update dependency go to v1.24.6 ([#1414](https://github.com/grafana/synthetic-monitoring-agent/issues/1414)) ([aa32474](https://github.com/grafana/synthetic-monitoring-agent/commit/aa324745fafab572f594937433a682919a1f064f))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:138.0.7204.157-r0-3.22.1 Docker digest to 340d57b ([8e297e9](https://github.com/grafana/synthetic-monitoring-agent/commit/8e297e9424b3ea6e60395ad7b8f8c883cb509aaf))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.12.0 ([247e054](https://github.com/grafana/synthetic-monitoring-agent/commit/247e054421fa96fa7d9039b4fafc43d0491ccd35))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.12.1 ([1ff879d](https://github.com/grafana/synthetic-monitoring-agent/commit/1ff879dac88842089b822826188f3a5451f981b1))
* Update grafana/shared-workflows digest to 53e1f52 ([#1409](https://github.com/grafana/synthetic-monitoring-agent/issues/1409)) ([5c1aae4](https://github.com/grafana/synthetic-monitoring-agent/commit/5c1aae47ab7161088e284e1e72a96d92e208279e))
* Update module github.com/miekg/dns to v1.1.68 ([62e260c](https://github.com/grafana/synthetic-monitoring-agent/commit/62e260c3cb584885194193db3e7fe6d2e7d3bc78))
* Update module golang.org/x/net to v0.43.0 ([e495f01](https://github.com/grafana/synthetic-monitoring-agent/commit/e495f016f72f70b34c4e2b89cb4ad67b8f8ed954))

## [0.39.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.39.1...v0.39.2) (2025-07-31)


### Fixes

* k6runner/http: increase number of attempts recorded in metric ([9c35de0](https://github.com/grafana/synthetic-monitoring-agent/commit/9c35de05157fc9e36363282857a32fe21c27570f))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.5.12 ([371f16e](https://github.com/grafana/synthetic-monitoring-agent/commit/371f16eacfb02269cafaf043fd1265aa43fed2de))
* Update grafana/shared-workflows digest to fd77b75 ([45e1aa7](https://github.com/grafana/synthetic-monitoring-agent/commit/45e1aa77c45248f14d243f5b8bb006127109e3ae))

## [0.39.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.39.0...v0.39.1) (2025-07-28)


### Fixes

* build a .tar.gz file with bare binaries and attach it to releases ([834a053](https://github.com/grafana/synthetic-monitoring-agent/commit/834a05381519bacf8f282a21907664090ebf1118))
* include LICENSE tarballs, version-os-arch in name ([b0abe50](https://github.com/grafana/synthetic-monitoring-agent/commit/b0abe509cfe1bddd23fd62816de4605c5dc413db))


### Miscellaneous Chores

* ci: do not push browser image to GAR ([bffa211](https://github.com/grafana/synthetic-monitoring-agent/commit/bffa211415b917d4ab5dab8ab3d44ae9d8f36786))
* Update alpine Docker tag to v3.22.1 ([1a5391f](https://github.com/grafana/synthetic-monitoring-agent/commit/1a5391fcf4ac9f7601722e27a2c02ec50b8fcb6c))
* Update grafana/shared-workflows digest to 8c3f72c ([#1396](https://github.com/grafana/synthetic-monitoring-agent/issues/1396)) ([b54aba8](https://github.com/grafana/synthetic-monitoring-agent/commit/b54aba80eba74e3832d88539297a627eac9c4b65))

## [0.39.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.5...v0.39.0) (2025-07-23)


### Features

* Implement usage report client ([#1366](https://github.com/grafana/synthetic-monitoring-agent/issues/1366)) ([d3c3c88](https://github.com/grafana/synthetic-monitoring-agent/commit/d3c3c889fe583126564825bd8c97d18a076ba6c1))


### Fixes

* k6: do not log k6 script contents before execution ([7c993e4](https://github.com/grafana/synthetic-monitoring-agent/commit/7c993e46e85e3abbf3af680ec4ea353030f48d58))
* runner: report script errors in the `RunResponse` object ([6f73c03](https://github.com/grafana/synthetic-monitoring-agent/commit/6f73c03566deaace2b2770949cac1c536b63181f))


### Miscellaneous Chores

* Update dependency go to v1.24.5 ([#1373](https://github.com/grafana/synthetic-monitoring-agent/issues/1373)) ([f29e11e](https://github.com/grafana/synthetic-monitoring-agent/commit/f29e11e6e21b3770497fb4e51dc7ab1b1a367cb8))
* Update dependency grafana/xk6-sm to v0.5.10 ([8b2047f](https://github.com/grafana/synthetic-monitoring-agent/commit/8b2047f6d73eed6fb3b1c0f56736e77daf51c181))
* Update dependency grafana/xk6-sm to v0.5.11 ([c746738](https://github.com/grafana/synthetic-monitoring-agent/commit/c74673846cf79fa277784044e9bcda6e1820ef40))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v138.0.7204.157-r0-3.22.1 ([b6af5ac](https://github.com/grafana/synthetic-monitoring-agent/commit/b6af5aca0156edfde7f8d6efcd967c48be373047))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v138.0.7204.93-r0-3.22.1 ([6a9bfa8](https://github.com/grafana/synthetic-monitoring-agent/commit/6a9bfa8e93f187580a73ab4afb5e682875c5d319))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.10.0 ([a602437](https://github.com/grafana/synthetic-monitoring-agent/commit/a6024375d7fc47e45d73f0e35f8ff854be6b98d3))
* Update golang.org/x/exp digest to 645b1fa ([#1395](https://github.com/grafana/synthetic-monitoring-agent/issues/1395)) ([fd37086](https://github.com/grafana/synthetic-monitoring-agent/commit/fd37086086cb200d5be0e8a8e5a3b11948ad7d56))
* Update module github.com/KimMachineGun/automemlimit to v0.7.4 ([40440e2](https://github.com/grafana/synthetic-monitoring-agent/commit/40440e2e92f2f0cfcf5744f30c744c5d23fcefb7))
* Update module google.golang.org/grpc to v1.74.2 ([94a0485](https://github.com/grafana/synthetic-monitoring-agent/commit/94a0485d2338a6e309f6e261783d5569978f4bee))

## [0.38.5](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.4...v0.38.5) (2025-07-14)


### Fixes

* remove hardcoded blacklistIPs and blockHostnames for multihttp checks ([46dca02](https://github.com/grafana/synthetic-monitoring-agent/commit/46dca023216825d1c2706209e1deace0bd7f4a0b))
* Replace zerolog.Nop() with conditional logger ([d04d1bf](https://github.com/grafana/synthetic-monitoring-agent/commit/d04d1bf3dd45a9afd87f5a3c325d4e0b5d8f50a6))
* Update CODEOWNERS file to enable release-please workflow ([88d8087](https://github.com/grafana/synthetic-monitoring-agent/commit/88d80878cb39b57b97ddb4817c4768462c311981))


### Miscellaneous Chores

* renovate: replace fileMatch with managerFilePatterns ([4eacef1](https://github.com/grafana/synthetic-monitoring-agent/commit/4eacef1e5fd3a1133444fc4bdf703bb0f8432fa1))
* Update dependency grafana/xk6-sm to v0.5.8 ([af1f1ed](https://github.com/grafana/synthetic-monitoring-agent/commit/af1f1ed930e76e02376592e65769151f3ba41390))
* Update dependency grafana/xk6-sm to v0.5.8 ([1d1922a](https://github.com/grafana/synthetic-monitoring-agent/commit/1d1922a0dbd6c15902085ced73a5a88e7ea3667d))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:138.0.7204.93-r0-3.22.0 Docker digest to 342d53b ([bd61f44](https://github.com/grafana/synthetic-monitoring-agent/commit/bd61f4461d8a18840fde64cc69bb2b25bf9ec037))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.9.0 ([e0f2050](https://github.com/grafana/synthetic-monitoring-agent/commit/e0f2050571e0d787108e9e49acda79a49925d54d))
* Update golang.org/x/exp digest to 6ae5c78 ([b109f76](https://github.com/grafana/synthetic-monitoring-agent/commit/b109f7631508fe4896022e97435ab4400da77731))
* Update grafana/shared-workflows digest to 388e3e9 ([49c9720](https://github.com/grafana/synthetic-monitoring-agent/commit/49c9720e509e43ebf3ef7046771b59aa59cee419))
* Update grafana/shared-workflows digest to e9a3e88 ([c991f74](https://github.com/grafana/synthetic-monitoring-agent/commit/c991f74e0895465340327f3a2f848e4916fc4a56))
* Update module github.com/miekg/dns to v1.1.67 ([7c7ca12](https://github.com/grafana/synthetic-monitoring-agent/commit/7c7ca12b7b15cc4a657993458bb005ec9c1e628a))
* Update module golang.org/x/sync to v0.16.0 ([f05ee0c](https://github.com/grafana/synthetic-monitoring-agent/commit/f05ee0ce9e5913ffdcaa71fd50f4f70b113622be))

## [0.38.4](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.3...v0.38.4) (2025-07-07)


### Fixes

* Redact password field in RemoteInfo type ([#1322](https://github.com/grafana/synthetic-monitoring-agent/issues/1322)) ([757d3c9](https://github.com/grafana/synthetic-monitoring-agent/commit/757d3c92080b40085219e11899a503f7522b1034))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.5.7 ([c68ca0a](https://github.com/grafana/synthetic-monitoring-agent/commit/c68ca0a220e0aafb61e9fb81c7709577ee193b0b))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1.8.0 ([#1367](https://github.com/grafana/synthetic-monitoring-agent/issues/1367)) ([e637d64](https://github.com/grafana/synthetic-monitoring-agent/commit/e637d642f231684e8789edc6b8f7b68721443d91))
* Update grafana/shared-workflows digest to 00646b3 ([aefdf8e](https://github.com/grafana/synthetic-monitoring-agent/commit/aefdf8e0933e9b3592a63fd6e0e1898d49ebe289))
* Update grafana/shared-workflows digest to 8717df3 ([#1368](https://github.com/grafana/synthetic-monitoring-agent/issues/1368)) ([b337f87](https://github.com/grafana/synthetic-monitoring-agent/commit/b337f879a9195181c58bb4c953d0722f0a3468c9))
* Update prometheus-go ([d5347f6](https://github.com/grafana/synthetic-monitoring-agent/commit/d5347f6630c5d96f8ec763d0d209c180d4a7b3e7))

## [0.38.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.2...v0.38.3) (2025-07-02)


### Miscellaneous Chores

* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v138 ([6c52dfb](https://github.com/grafana/synthetic-monitoring-agent/commit/6c52dfb70a4c7d65a85b9e0c88ef62588ebb73b6))

## [0.38.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.1...v0.38.2) (2025-06-30)


### Fixes

* Don't stop publisher if we hit Loki's stream limit ([#1356](https://github.com/grafana/synthetic-monitoring-agent/issues/1356)) ([917541c](https://github.com/grafana/synthetic-monitoring-agent/commit/917541c48e410a8a895b29f8143f91705c0bbf69))


### Miscellaneous Chores

* Update golang.org/x/exp digest to b7579e2 ([#1190](https://github.com/grafana/synthetic-monitoring-agent/issues/1190)) ([1755745](https://github.com/grafana/synthetic-monitoring-agent/commit/1755745eef7967387cd10e84742ef913cb331c4e))

## [0.38.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.38.0...v0.38.1) (2025-06-20)


### Miscellaneous Chores

* k6runner: replace exp/rand with math/rand/v2 ([#1325](https://github.com/grafana/synthetic-monitoring-agent/issues/1325)) ([73f4af7](https://github.com/grafana/synthetic-monitoring-agent/commit/73f4af7944dceeecb503de13b605450f7c719acc))
* Update docker/setup-buildx-action action to v3.11.1 ([#1348](https://github.com/grafana/synthetic-monitoring-agent/issues/1348)) ([56ad9e0](https://github.com/grafana/synthetic-monitoring-agent/commit/56ad9e0afdbfb4dfed46250afc86c79ff0e91b9b))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v137.0.7151.103-r0-edge ([d329f31](https://github.com/grafana/synthetic-monitoring-agent/commit/d329f312017f85ae256cc63f4f323955456ec37b))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:137.0.7151.68-r0-3.22.0 Docker digest to d1b6268 ([#1347](https://github.com/grafana/synthetic-monitoring-agent/issues/1347)) ([9d1e880](https://github.com/grafana/synthetic-monitoring-agent/commit/9d1e880610b6f330aafd4bc922fc54852f049c82))
* Update grafana/shared-workflows digest to d24aec3 ([#1343](https://github.com/grafana/synthetic-monitoring-agent/issues/1343)) ([3145981](https://github.com/grafana/synthetic-monitoring-agent/commit/3145981bb1cd45ef982215c2d3e6306ed03ebb2e))
* Update module github.com/golang/snappy to v1 ([#1249](https://github.com/grafana/synthetic-monitoring-agent/issues/1249)) ([36a02d6](https://github.com/grafana/synthetic-monitoring-agent/commit/36a02d66a6230fb33360c515c0b54b1499658f64))

## [0.38.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.7...v0.38.0) (2025-06-17)


### Features

* Add enableProtocolSecrets and supportsProtocolSecrets to protobuf ([#1344](https://github.com/grafana/synthetic-monitoring-agent/issues/1344)) ([6fa78f5](https://github.com/grafana/synthetic-monitoring-agent/commit/6fa78f5ad93ca978e611622ba044f81e0f8732c5))


### Miscellaneous Chores

* ci: pin exact version of actions/github-create-app-token ([693a1e2](https://github.com/grafana/synthetic-monitoring-agent/commit/693a1e2563416c27f86f4ff97d0e0133f018db49))
* Update dependency grafana/xk6-sm to v0.5.6 ([#1340](https://github.com/grafana/synthetic-monitoring-agent/issues/1340)) ([b328862](https://github.com/grafana/synthetic-monitoring-agent/commit/b328862b984bed226cffa14b50a2ee83bddc9ee9))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v137 ([#1334](https://github.com/grafana/synthetic-monitoring-agent/issues/1334)) ([b0364c4](https://github.com/grafana/synthetic-monitoring-agent/commit/b0364c46cea8c43579017a71367b943e04957078))
* Update ghcr.io/grafana/chromium-swiftshader-alpine:137.0.7151.68-r0-3.22.0 Docker digest to 5f6a1fc ([756c46d](https://github.com/grafana/synthetic-monitoring-agent/commit/756c46d2e1c3fcf2de90fb9d91738723120ef825))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v1 ([1116688](https://github.com/grafana/synthetic-monitoring-agent/commit/111668860fa537a40fd1811d39000caabe714028))
* Update grafana/shared-workflows digest to cb40def ([#1337](https://github.com/grafana/synthetic-monitoring-agent/issues/1337)) ([ddc5ee0](https://github.com/grafana/synthetic-monitoring-agent/commit/ddc5ee03f12df7429195c1b08bcf07cf50ebd4c7))
* Update module github.com/KimMachineGun/automemlimit to v0.7.3 ([9130943](https://github.com/grafana/synthetic-monitoring-agent/commit/9130943e10601f05ab9583010b279f0bc77deb02))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.18.2 ([#1339](https://github.com/grafana/synthetic-monitoring-agent/issues/1339)) ([7d1393d](https://github.com/grafana/synthetic-monitoring-agent/commit/7d1393d5c51650a82e37bb039014acdca58d4c83))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.18.3 ([fc7724a](https://github.com/grafana/synthetic-monitoring-agent/commit/fc7724a049a669e597aeddf0d74d41f68341572e))

## [0.37.7](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.6...v0.37.7) (2025-06-06)


### Miscellaneous Chores

* Register Component into Software Catalog and set up TechDocs publishing ([#1319](https://github.com/grafana/synthetic-monitoring-agent/issues/1319)) ([8bb2518](https://github.com/grafana/synthetic-monitoring-agent/commit/8bb2518b7baee64af4f864acc1776d2a8d9520dc))
* Update actions/create-github-app-token action to v2 ([5ac2c54](https://github.com/grafana/synthetic-monitoring-agent/commit/5ac2c549df7c1aa7fb4aed8b429b39f931d84721))
* Update actions/download-artifact digest to d3f86a1 ([4b06427](https://github.com/grafana/synthetic-monitoring-agent/commit/4b064277d6ab14702d6f3dc5e152615d514b7437))
* Update actions/setup-go action to v5.5.0 ([2ec4a13](https://github.com/grafana/synthetic-monitoring-agent/commit/2ec4a13d31a59bddd44d35d2fdc3b4a39081db5a))
* Update alpine Docker tag to v3.22.0 ([9963d22](https://github.com/grafana/synthetic-monitoring-agent/commit/9963d229a0c4f5e9e7b857609903536eddb25578))
* Update dependency go to v1.24.4 ([1f1ecc2](https://github.com/grafana/synthetic-monitoring-agent/commit/1f1ecc2950235a0b1980924214d043b7451b15f6))
* Update docker/build-push-action action to v6.18.0 ([79737b0](https://github.com/grafana/synthetic-monitoring-agent/commit/79737b0b7090601967b19e548f4f5871b6f4425e))
* Update docker/setup-buildx-action action to v3.10.0 ([30067e6](https://github.com/grafana/synthetic-monitoring-agent/commit/30067e6dd03b05dc4d691c3cea4ee1a74fc4c7d6))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v136 ([488e51c](https://github.com/grafana/synthetic-monitoring-agent/commit/488e51cb1737d5c1aa78ebad500ac59d6111ab0b))
* Update module github.com/KimMachineGun/automemlimit to v0.7.2 ([68cb8f4](https://github.com/grafana/synthetic-monitoring-agent/commit/68cb8f4cd4bb1972578a918eeaedf366b1c6a2bf))
* Update module github.com/miekg/dns to v1.1.66 ([fcf7cf3](https://github.com/grafana/synthetic-monitoring-agent/commit/fcf7cf3a4dbfe2b828f222669a28ac49484def61))
* Update module golang.org/x/net to v0.41.0 ([4f4c747](https://github.com/grafana/synthetic-monitoring-agent/commit/4f4c747b09072de7aaf40412f0b86456a46107ff))
* Update module google.golang.org/grpc to v1.73.0 ([df12825](https://github.com/grafana/synthetic-monitoring-agent/commit/df128256d156bb9a5fd25ccf384afa4baea7c310))

## [0.37.6](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.5...v0.37.6) (2025-05-20)


### Fixes

* deployment: add commented-out mountpoint for `/home/sm` (browser) ([eed6231](https://github.com/grafana/synthetic-monitoring-agent/commit/eed62310273f948a4e7f67d4a035c17d7669011e))
* revert the tenant cache to 15 minutes ([#1316](https://github.com/grafana/synthetic-monitoring-agent/issues/1316)) ([f54d4c5](https://github.com/grafana/synthetic-monitoring-agent/commit/f54d4c5c5f22f2678a369595efc5e1d37c3554c7))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.5.5 ([#1314](https://github.com/grafana/synthetic-monitoring-agent/issues/1314)) ([9fb9461](https://github.com/grafana/synthetic-monitoring-agent/commit/9fb94615d2db50cf8bf1e7c689d4ec0f62adc3b0))
* Update module google.golang.org/grpc to v1.72.1 ([#1306](https://github.com/grafana/synthetic-monitoring-agent/issues/1306)) ([1ffd451](https://github.com/grafana/synthetic-monitoring-agent/commit/1ffd451a84de2f098e9a135b622fa1b4104af182))
* Update push-to-gar-docker action to v0.4.1 ([#1315](https://github.com/grafana/synthetic-monitoring-agent/issues/1315)) ([a8309d5](https://github.com/grafana/synthetic-monitoring-agent/commit/a8309d59215881d0ac49464b425f88ae313c33ab))
* upgrade blackbox exporter ([#1255](https://github.com/grafana/synthetic-monitoring-agent/issues/1255)) ([fcd3eed](https://github.com/grafana/synthetic-monitoring-agent/commit/fcd3eed990ca14fd863dae8ed39d572bd2c8df43))

## [0.37.5](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.4...v0.37.5) (2025-05-06)


### Miscellaneous Chores

* Update module github.com/spf13/afero to v1.14.0 ([56921fd](https://github.com/grafana/synthetic-monitoring-agent/commit/56921fd411e54669f45265fd4f9c38b0ccf4a57c))

## [0.37.4](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.3...v0.37.4) (2025-05-05)


### Miscellaneous Chores

* release 0.37.4 ([#1311](https://github.com/grafana/synthetic-monitoring-agent/issues/1311)) ([b2ee2d7](https://github.com/grafana/synthetic-monitoring-agent/commit/b2ee2d71a3adb36b5d250c15cdcb17e673f3e874))

## [0.37.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.2...v0.37.3) (2025-05-05)


### Fixes

* Adjust workflow permissions ([#1307](https://github.com/grafana/synthetic-monitoring-agent/issues/1307)) ([828b6e1](https://github.com/grafana/synthetic-monitoring-agent/commit/828b6e137a029333404b5d78d65d7de0b9118cd1))

## [0.37.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.1...v0.37.2) (2025-04-30)


### Fixes

* Resolve issues reported by zizmor ([#1302](https://github.com/grafana/synthetic-monitoring-agent/issues/1302)) ([a76f949](https://github.com/grafana/synthetic-monitoring-agent/commit/a76f949224ba23a6cb85834845b632310d6ae835))
* Resolve issues reported by zizmor, part 2 ([#1303](https://github.com/grafana/synthetic-monitoring-agent/issues/1303)) ([5b34259](https://github.com/grafana/synthetic-monitoring-agent/commit/5b3425947486ea8faa7bd658402eefff436e9ab4))


### Miscellaneous Chores

* Update dependency grafana/xk6-sm to v0.5.3 ([#1301](https://github.com/grafana/synthetic-monitoring-agent/issues/1301)) ([4c7b7c8](https://github.com/grafana/synthetic-monitoring-agent/commit/4c7b7c8631c2bc7a8f1be176e69e81d638baf694))
* Update module github.com/prometheus-community/pro-bing to v0.7.0 ([#1290](https://github.com/grafana/synthetic-monitoring-agent/issues/1290)) ([bd38d05](https://github.com/grafana/synthetic-monitoring-agent/commit/bd38d058b1f1a345e081aea6e9ac7251c8d1efe4))
* Update module github.com/rs/zerolog to v1.34.0 ([#1291](https://github.com/grafana/synthetic-monitoring-agent/issues/1291)) ([d014686](https://github.com/grafana/synthetic-monitoring-agent/commit/d01468662f4fc026921d18a2ed6329dcd29932e6))
* Update module kernel.org/pub/linux/libs/security/libcap/cap to v1.2.76 ([#1298](https://github.com/grafana/synthetic-monitoring-agent/issues/1298)) ([fcc0d07](https://github.com/grafana/synthetic-monitoring-agent/commit/fcc0d07f9f136b8dcc4aaf6f40be1be2a0bdff10))

## [0.37.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.37.0...v0.37.1) (2025-04-17)


### Miscellaneous Chores

* Update module golang.org/x/net to v0.38.0 [SECURITY] ([#1296](https://github.com/grafana/synthetic-monitoring-agent/issues/1296)) ([63ca400](https://github.com/grafana/synthetic-monitoring-agent/commit/63ca40032d31c4e5713e2cee16cf4c233fbcdd12))

## [0.37.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.36.0...v0.37.0) (2025-04-10)


### Features

* bust the tenant cache on the token expiry ([#1279](https://github.com/grafana/synthetic-monitoring-agent/issues/1279)) ([371fe13](https://github.com/grafana/synthetic-monitoring-agent/commit/371fe135ee8ea70898aeae34a04491df79f73077))


### Fixes

* Change how the secret store is handled ([#1281](https://github.com/grafana/synthetic-monitoring-agent/issues/1281)) ([5a78dae](https://github.com/grafana/synthetic-monitoring-agent/commit/5a78dae0a786a8d9ceb5c6d03cb8e8c667c882a5))
* Remove second k6 binary for gsm ([#1274](https://github.com/grafana/synthetic-monitoring-agent/issues/1274)) ([dedd951](https://github.com/grafana/synthetic-monitoring-agent/commit/dedd951f97815c748bc12a24def5169e64c18d5c))
* Subtract the timeout from the token expiry, to ensure it will last for the duration ([#1282](https://github.com/grafana/synthetic-monitoring-agent/issues/1282)) ([773f7a3](https://github.com/grafana/synthetic-monitoring-agent/commit/773f7a3d70e07882756ed78dcc557bf43e7a835e))
* Synchronize makefiles with more recent versions ([#1214](https://github.com/grafana/synthetic-monitoring-agent/issues/1214)) ([7e124e8](https://github.com/grafana/synthetic-monitoring-agent/commit/7e124e85193f234b3e58e6e4966c0ae341fce856))
* Update policybot config ([3a0b7ca](https://github.com/grafana/synthetic-monitoring-agent/commit/3a0b7cad255fa0d06b24b1557083b7b7a7dd5b63))


### Miscellaneous Chores

* examples: mount an emptydir volume in /tmp ([009ff5f](https://github.com/grafana/synthetic-monitoring-agent/commit/009ff5ff7fbb7f25fdb5645425b09ba8c7290a14))
* Update actions/cache action to v4.2.3 ([#1275](https://github.com/grafana/synthetic-monitoring-agent/issues/1275)) ([6a5b781](https://github.com/grafana/synthetic-monitoring-agent/commit/6a5b781c786ca9eb1675bf7d55fc5bad6ad78a0f))
* Update actions/download-artifact digest to 95815c3 ([#1256](https://github.com/grafana/synthetic-monitoring-agent/issues/1256)) ([87c6d21](https://github.com/grafana/synthetic-monitoring-agent/commit/87c6d21a3c8bf38a85784ccee0e9db2d2ece80d7))
* Update actions/setup-go action to v5.4.0 ([#1289](https://github.com/grafana/synthetic-monitoring-agent/issues/1289)) ([93e94da](https://github.com/grafana/synthetic-monitoring-agent/commit/93e94da807683ce5881c29bc9b8e55fed673ca63))
* Update dependency go to v1.24.2 ([#1285](https://github.com/grafana/synthetic-monitoring-agent/issues/1285)) ([4c30f60](https://github.com/grafana/synthetic-monitoring-agent/commit/4c30f6055ce55e226ce99758749f3e4f8b118b92))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.18.1 ([#1286](https://github.com/grafana/synthetic-monitoring-agent/issues/1286)) ([3ce93fd](https://github.com/grafana/synthetic-monitoring-agent/commit/3ce93fdc5c7a79e5a55e53cbd2926af14d9e3f0b))
* Update module github.com/miekg/dns to v1.1.65 ([#1287](https://github.com/grafana/synthetic-monitoring-agent/issues/1287)) ([20cef71](https://github.com/grafana/synthetic-monitoring-agent/commit/20cef7133afc48ecdcf48bd17b0d9f60a45a94a2))
* Update module google.golang.org/grpc to v1.71.1 ([#1288](https://github.com/grafana/synthetic-monitoring-agent/issues/1288)) ([f36e667](https://github.com/grafana/synthetic-monitoring-agent/commit/f36e667ab7f4c0d8e24874b301aa09a251cd8e4a))
* Update module gotest.tools/gotestsum to v1.12.1 ([#1254](https://github.com/grafana/synthetic-monitoring-agent/issues/1254)) ([db2c8c2](https://github.com/grafana/synthetic-monitoring-agent/commit/db2c8c265fc1ba356e725535f6a35d47664ecfe8))
* Update module kernel.org/pub/linux/libs/security/libcap/cap to v1.2.75 ([#1242](https://github.com/grafana/synthetic-monitoring-agent/issues/1242)) ([67c5eb3](https://github.com/grafana/synthetic-monitoring-agent/commit/67c5eb3d5608092403949e3bbcc8486c137a5ac3))

## [0.36.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.35.2...v0.36.0) (2025-04-03)


### Features

* report metrics about requests made to remote runners ([8f93e87](https://github.com/grafana/synthetic-monitoring-agent/commit/8f93e87fbdb9e44e069bb3de504cf6eb00bd656e))


### Fixes

* Add a safety net in adhoc's defaultRunnerFactory ([#1268](https://github.com/grafana/synthetic-monitoring-agent/issues/1268)) ([0568a50](https://github.com/grafana/synthetic-monitoring-agent/commit/0568a500f0f5d9f7afee5fb8968f8512ad89e642))
* Make test reporting style configurable ([#1261](https://github.com/grafana/synthetic-monitoring-agent/issues/1261)) ([793c4ea](https://github.com/grafana/synthetic-monitoring-agent/commit/793c4eaa4e7fecdfe8c04c0488f64030ef879cac))
* Update Go version retrieval logic ([#1262](https://github.com/grafana/synthetic-monitoring-agent/issues/1262)) ([87d4781](https://github.com/grafana/synthetic-monitoring-agent/commit/87d47817758e1257f884945811e83efbef9d9364))


### Miscellaneous Chores

* ci: remove unused `Renovate reviewer` workflow ([2699f62](https://github.com/grafana/synthetic-monitoring-agent/commit/2699f62001b70a540d2d8f0132919cc8de0409ba))
* runner/test: initialize metrics with a noop registry ([de0b714](https://github.com/grafana/synthetic-monitoring-agent/commit/de0b714c6385207d069d285c60d37b936e76b0fb))
* Update actions/create-github-app-token digest to d72941d ([8623d6d](https://github.com/grafana/synthetic-monitoring-agent/commit/8623d6d351cc430dfcc17379f37efbfd528f4f2b))

## [0.35.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.35.1...v0.35.2) (2025-03-27)


### Miscellaneous Chores

* **deps:** bump github.com/mccutchen/go-httpbin/v2 ([be4f4f0](https://github.com/grafana/synthetic-monitoring-agent/commit/be4f4f01881aaf48becf48e54560666ca8dc5e42))
* Update actions/upload-artifact digest to ea165f8 ([b7587fa](https://github.com/grafana/synthetic-monitoring-agent/commit/b7587fab2bc452c8c6a176d43aa1b8f2eb843801))

## [0.35.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.35.0...v0.35.1) (2025-03-25)


### Fixes

* Pass Tenant ID to adhoc checks ([#1267](https://github.com/grafana/synthetic-monitoring-agent/issues/1267)) ([8255fa3](https://github.com/grafana/synthetic-monitoring-agent/commit/8255fa3152703e902c2c4d5e8914ea7f7c796607))
* Report errors when retrieving secrets ([#1265](https://github.com/grafana/synthetic-monitoring-agent/issues/1265)) ([39d80d1](https://github.com/grafana/synthetic-monitoring-agent/commit/39d80d12ef67c2bd1833dde7a58d9d6008e8d32f))

## [0.35.0](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.34.4...v0.35.0) (2025-03-21)


### Features

* Add the sm-k6-gsm binary for secrets ([#1234](https://github.com/grafana/synthetic-monitoring-agent/issues/1234)) ([94a3d29](https://github.com/grafana/synthetic-monitoring-agent/commit/94a3d290d34de1fd449e6ef4036855ffbe4a660c))
* pass secret store from API ([#1179](https://github.com/grafana/synthetic-monitoring-agent/issues/1179)) ([fcfcd8f](https://github.com/grafana/synthetic-monitoring-agent/commit/fcfcd8f3564a7645f380764ad2ca03afc22decdd))


### Fixes

* Update version reporting logic ([#1244](https://github.com/grafana/synthetic-monitoring-agent/issues/1244)) ([ad3ffbb](https://github.com/grafana/synthetic-monitoring-agent/commit/ad3ffbbecf44565f74aefedc5dd3a91b6d138ad2))
* Use a global tenant ID when retrieving secrets information ([#1260](https://github.com/grafana/synthetic-monitoring-agent/issues/1260)) ([f171fc4](https://github.com/grafana/synthetic-monitoring-agent/commit/f171fc456d6b8a4da0a46647c1ac32c61ee579a2))


### Miscellaneous Chores

* examples/kubernetes: default to a hardened `securityContext` ([18b6df0](https://github.com/grafana/synthetic-monitoring-agent/commit/18b6df076ebc1506495d494778698a07df82bc52))
* Update dependency grafana/xk6-sm to v0.2.0 ([#1161](https://github.com/grafana/synthetic-monitoring-agent/issues/1161)) ([b56c7a0](https://github.com/grafana/synthetic-monitoring-agent/commit/b56c7a00a6fe5d2d727e89ace0fd217be036018b))
* Update grafana/shared-workflows digest to 68b38bb ([047466f](https://github.com/grafana/synthetic-monitoring-agent/commit/047466f31788ca01e20732079c72d2d7506d8612))
* Update module github.com/mccutchen/go-httpbin/v2 to v2.17.1 ([#1240](https://github.com/grafana/synthetic-monitoring-agent/issues/1240)) ([91c5550](https://github.com/grafana/synthetic-monitoring-agent/commit/91c5550d2d4f64c444540e3986b674fd15e1f224))
* Update module github.com/securego/gosec/v2 to v2.22.2 ([3977d87](https://github.com/grafana/synthetic-monitoring-agent/commit/3977d87400513d59d76e7152269bc689dbc11e4b))
* Update module golang.org/x/net to v0.37.0 ([#1245](https://github.com/grafana/synthetic-monitoring-agent/issues/1245)) ([fed448b](https://github.com/grafana/synthetic-monitoring-agent/commit/fed448bf9cde3aa61644ab490e8ba7cf2a132e4f))
* Update module google.golang.org/grpc to v1.71.0 ([84aeea1](https://github.com/grafana/synthetic-monitoring-agent/commit/84aeea139b3af70711a5bb41507a2f884dfdcc5e))

## [0.34.4](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.34.3...v0.34.4) (2025-03-10)


### Miscellaneous Chores

* remove unused renovate-app.json ([1658ef7](https://github.com/grafana/synthetic-monitoring-agent/commit/1658ef72bd88de98b2169320cd2a72a7ca248e39))
* Update actions/create-github-app-token digest to 21cfef2 ([#1232](https://github.com/grafana/synthetic-monitoring-agent/issues/1232)) ([b4e310b](https://github.com/grafana/synthetic-monitoring-agent/commit/b4e310bdbb55a39f5c7cda631c6e0ed5916645bf))
* Update dependency go to v1.24.1 ([#1233](https://github.com/grafana/synthetic-monitoring-agent/issues/1233)) ([77b51bc](https://github.com/grafana/synthetic-monitoring-agent/commit/77b51bccf3c039bdbecfcf6526216a36f7bd26a1))
* Update docker/build-push-action action to v6.15.0 ([fac4f07](https://github.com/grafana/synthetic-monitoring-agent/commit/fac4f073bf9879b820de56e50b584939acc24ede))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v133.0.6943.141-r0-3.21.3 ([#1236](https://github.com/grafana/synthetic-monitoring-agent/issues/1236)) ([15bdd33](https://github.com/grafana/synthetic-monitoring-agent/commit/15bdd332133b2d81983f55461cbf1ff8973760a5))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.40.3 ([b9eca69](https://github.com/grafana/synthetic-monitoring-agent/commit/b9eca690b614b27d1434c0bc9117dcb51d49bd74))
* Update googleapis/release-please-action digest to a02a34c ([#1235](https://github.com/grafana/synthetic-monitoring-agent/issues/1235)) ([f04a7b4](https://github.com/grafana/synthetic-monitoring-agent/commit/f04a7b4677698bb943d15a85332c2c0ecdf4bf65))
* Update module github.com/golangci/golangci-lint to v1.64.6 ([#1230](https://github.com/grafana/synthetic-monitoring-agent/issues/1230)) ([5af16db](https://github.com/grafana/synthetic-monitoring-agent/commit/5af16dbd714d61d9e9c5c2ec7b852f9f71dfcfe1))
* Update module github.com/KimMachineGun/automemlimit to v0.7.1 ([#1218](https://github.com/grafana/synthetic-monitoring-agent/issues/1218)) ([c08a537](https://github.com/grafana/synthetic-monitoring-agent/commit/c08a537b402fd2f1b1e9f01263427cfbeae81c20))

## [0.34.3](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.34.2...v0.34.3) (2025-02-28)


### Miscellaneous Chores

* Update actions/cache action to v4.2.2 ([#1227](https://github.com/grafana/synthetic-monitoring-agent/issues/1227)) ([1b3eb9e](https://github.com/grafana/synthetic-monitoring-agent/commit/1b3eb9e9d3e253bd2552430c0a35099fef02b11d))
* Update actions/download-artifact digest to cc20338 ([#1223](https://github.com/grafana/synthetic-monitoring-agent/issues/1223)) ([4fcb926](https://github.com/grafana/synthetic-monitoring-agent/commit/4fcb92621f69b10b50871b201720142916ce4b4c))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v133 ([ac67a05](https://github.com/grafana/synthetic-monitoring-agent/commit/ac67a055261faa23b073cd544282b1aa6feb0605))
* Update grafana/shared-workflows digest to a7bc536 ([40af603](https://github.com/grafana/synthetic-monitoring-agent/commit/40af60339495e57174c49d45932c3b9210042920))

## [0.34.2](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.34.1...v0.34.2) (2025-02-24)


### Miscellaneous Chores

* golangci: remove properties rejected by schema ([#1209](https://github.com/grafana/synthetic-monitoring-agent/issues/1209)) ([490936c](https://github.com/grafana/synthetic-monitoring-agent/commit/490936caf0ff090410fe86836855515ad92934bb))
* Update actions/cache action to v4.2.1 ([d180329](https://github.com/grafana/synthetic-monitoring-agent/commit/d180329e2a47cee879c9be9551e3d293a2660592))
* Update actions/upload-artifact digest to 4cec3d8 ([#1217](https://github.com/grafana/synthetic-monitoring-agent/issues/1217)) ([48b9ae7](https://github.com/grafana/synthetic-monitoring-agent/commit/48b9ae70674cf6764745d8c48ea0a7a8b1395a5f))
* Update docker/build-push-action action to v6.14.0 ([7f5053e](https://github.com/grafana/synthetic-monitoring-agent/commit/7f5053ecd42d5cb74b43b28f647dbc3ad14444dd))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.40.1 ([f7cbf1c](https://github.com/grafana/synthetic-monitoring-agent/commit/f7cbf1c23588467deb078461ba950914189f5ca3))
* Update module github.com/dmarkham/enumer to v1.5.11 ([e65ba61](https://github.com/grafana/synthetic-monitoring-agent/commit/e65ba613789914ea08ac99a532675790d2f79046))
* Update prometheus-go ([#1208](https://github.com/grafana/synthetic-monitoring-agent/issues/1208)) ([2bc2855](https://github.com/grafana/synthetic-monitoring-agent/commit/2bc285523b011ebf6e0c41f53f50a24d33f96cf6))

## [0.34.1](https://github.com/grafana/synthetic-monitoring-agent/compare/v0.34.0...v0.34.1) (2025-02-17)


### Miscellaneous Chores

* golangcilint: disable `exportloopref` linter ([adda550](https://github.com/grafana/synthetic-monitoring-agent/commit/adda550e0fbc38211b4aafc85c6e9b4f3ebcf920))
* Update actions/create-github-app-token digest to 0d56448 ([97ac799](https://github.com/grafana/synthetic-monitoring-agent/commit/97ac7995994c85875273e9782d8f57233906abc3))
* Update alpine Docker tag to v3.21.3 ([7e05333](https://github.com/grafana/synthetic-monitoring-agent/commit/7e053337442cba4cd793acc289004167b6cf8a7d))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v132 ([#1175](https://github.com/grafana/synthetic-monitoring-agent/issues/1175)) ([0f1e413](https://github.com/grafana/synthetic-monitoring-agent/commit/0f1e4136a18420758a0cd02bc81d428df4321b4f))
* Update ghcr.io/grafana/chromium-swiftshader-alpine Docker tag to v132.0.6834.83-r0-3.21.3 ([ac6f03d](https://github.com/grafana/synthetic-monitoring-agent/commit/ac6f03de9f1081baa6e96fa139d24f1239acc0f2))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.39.0 ([87352b1](https://github.com/grafana/synthetic-monitoring-agent/commit/87352b1c244e6fb2f9eff89b434c550bbe1c2d2d))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.39.1 ([#1199](https://github.com/grafana/synthetic-monitoring-agent/issues/1199)) ([2b85ab9](https://github.com/grafana/synthetic-monitoring-agent/commit/2b85ab9b82efb1915bc976b7e6367a736a291580))
* Update ghcr.io/grafana/grafana-build-tools Docker tag to v0.40.0 ([4ab358c](https://github.com/grafana/synthetic-monitoring-agent/commit/4ab358c70a7a0dc1269ca5edbab72166271ee199))
* Update grafana/shared-workflows digest to 56d0989 ([98db8c5](https://github.com/grafana/synthetic-monitoring-agent/commit/98db8c5b32a5b66a57bd28ace7a45da0563ed4cd))
* Update grafana/shared-workflows digest to 7f3871d ([8de8207](https://github.com/grafana/synthetic-monitoring-agent/commit/8de820741754a17b0aeadedaab3b9e9df1b1ae3a))
* Update module github.com/golangci/golangci-lint to v1.64.4 ([0fe816c](https://github.com/grafana/synthetic-monitoring-agent/commit/0fe816c414c2983de4ea6560a55939798293c829))
* Update module github.com/golangci/golangci-lint to v1.64.5 ([#1201](https://github.com/grafana/synthetic-monitoring-agent/issues/1201)) ([6cb293b](https://github.com/grafana/synthetic-monitoring-agent/commit/6cb293ba8c7063d634c99f47af500e3b1325a593))

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
