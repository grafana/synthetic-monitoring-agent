<a name="unreleased"></a>
## [Unreleased]


<a name="v0.24.3"></a>
## [v0.24.3] - 2024-06-18
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
- upgrade k6 to v0.50.0 ([#681](https://github.com/grafana/synthetic-monitoring-agent/issues/681))

### Fix
- use uniform timeout validation logic ([#693](https://github.com/grafana/synthetic-monitoring-agent/issues/693))
- TestTickWithOffset sometimes if offset is 0 ([#686](https://github.com/grafana/synthetic-monitoring-agent/issues/686))

### K6runner
- inspect errors and propagate unexpected ones to the probe
- handle errors reported by http runners


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

[Unreleased]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.3...HEAD
[v0.24.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.2...v0.24.3
[v0.24.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.1...v0.24.2
[v0.24.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.24.0...v0.24.1
[v0.24.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.3...v0.24.0
[v0.23.3]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.2...v0.23.3
[v0.23.2]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.1...v0.23.2
[v0.23.1]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.23.0...v0.23.1
[v0.23.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.22.0...v0.23.0
[v0.22.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.21.0...v0.22.0
[v0.21.0]: https://github.com/grafana/synthetic-monitoring-agent/compare/v0.19.6...v0.21.0
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
