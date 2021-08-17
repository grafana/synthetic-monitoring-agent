# Drone's Configuration

The configuration is signed to prevent tampering from non-Grafana employees ([ref](https://github.com/grafana/deployment_tools/blob/master/docs/infrastructure/drone/signing.md)).

To modify the Drone definition:

1. Modify the `drone.jsonnet` file. The `drone.yml` file is generated
2. [Export your Drone token here](https://drone.grafana.net/account)
3. Generate with the `make drone` command
