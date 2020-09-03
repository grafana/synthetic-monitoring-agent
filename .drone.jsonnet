local step(name, commands, image='circleci/golang:1.13.10') = {
  name: name,
  commands: commands,
  image: image,
};

local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  steps: [step('runner identification', ['echo $DRONE_RUNNER_NAME'], 'alpine')] + steps,
};

local masterOnly = {
  when: {branch:['master']},
};

local repo = 'grafana/synthetic-monitoring-agent';

local vault_secret(name, vault_path, key) = {
  kind: 'secret',
  name: name,
  get: {
    path: vault_path,
    name: key,
  },
};

[
  pipeline('build', [
    step('lint', ['make lint']),
    step('test', ['make test']),
    step('build', [
      'git fetch origin --tags',
      './scripts/version',
      './scripts/version > .tags', // save version in special file for docker plugin
      'make build',
    ]),
    // We can't use 'make docker' without making this repo priveleged in drone
    // so we will use the native docker plugin instead for security.
    step('docker build',[],'plugins/docker')+{
      settings:{
        repo: repo,
        dry_run: 'true',
      }
    },
    step('docker push',[],'plugins/docker')
    + {
        settings:{
          repo: repo,
          username: {from_secret: 'docker_username'},
          password: {from_secret: 'docker_password'},
        }
    }
    + masterOnly,
    step('package', ['make package']),
  ]),

  vault_secret('docker_username','infra/data/ci/docker_hub', 'username'),
  vault_secret('docker_password','infra/data/ci/docker_hub', 'password'),

]