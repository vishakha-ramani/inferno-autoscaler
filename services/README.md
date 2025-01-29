# Control loop

![control-loop](../docs/arch/components.png)

## Demo

Steps to run a demo of the control loop:

- Create a Kubernetes cluster and make sure `$HOME/.kube/config` points to it.
- Run script to create terminals for the various components. (Hint: [Change OSX Terminal Settings from Command Line](https://ict4g.net/adolfo/notes/admin/change-osx-terminal-settings-from-command-line.html))

    ```bash
    cd scripts
    ./launch-terms.sh
    ```

    ![snapshot](../docs/arch/snapshot.png)

    There are five components: Collector, Optimizer, Actuator, Controller, and Load Generator.
    Terminals for the Collector, Optimizer, Actuator, and Controller are (light) green, red, blue, and red, repectively.
    The Load Generator is orange.
    The green terminal is for kubectl commands.
    And, the beige terminal to show the currently runnning pods.

- Set the environment in the component terminals by running

    ```bash
    . $INFERNO_REPO/services/scripts/setenv.sh
    ```

    where `$INFERNO_REPO` is the path to the repository.

- Deploy sample deplyments (green terminal)

    ```bash
    kubectl deploy -f ns.yaml
    kubectl deploy -f dep1.yaml,dep2.yaml,dep3.yaml
    ```

    Three deployments will be created in the namespace `infer`.

- Start the components.
  