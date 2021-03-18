Synthetic Monitoring agent development
======================================

Building
--------

Everything is built using:

```
$ make
```

this will include verifying the Go modules, so it hits the network. If
you need to skip that step, you can use:

```
$ make build
```

The documentation for other targets is available thru:

```
$ make help
```

Linting
-------

Code linters are run by using:

```
$ make lint
```

Because this can be slow, linting is not run as part of default lint
target.

Tests
-----

All the tests are run using:

```
$ make test
```

Code generation
---------------

Some of the code and support files are generated, especifically:

- internal/scraper/testdata/*.txt
- pkg/accounting/data.go
- pkg/pb/synthetic_monitoring/checks.pb.go

In order to update `internal/scraper/testdata/*.txt` you can use the
`testdata` target in the Makefile.

`pkg/accounting/data.go` and `pkg/pb/synthetic_monitoring/checks.pb.go`
are updated by the `generate` target in the Makefile.

Code generation is *not* run automatically, but there are some tests
that try to detect discrepancies.

Architecture
------------

The agent obtains configuration information from the
synthetic-monitoring-api and uses it to build a configuration for
[blackbox_exporter](https://github.com/prometheus/blackbox_exporter),
which is how checks are executed.

![agent process][process]

[process]: https://www.planttext.com/api/plantuml/svg/dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0
[PlantUML]: https://www.planttext.com/?text=dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0
