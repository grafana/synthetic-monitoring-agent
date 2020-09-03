local step(name, commands, image) = {
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


[
  pipeline('build', [
    step('lint', ['make lint'], 'golang:1.15'),
  ])
]