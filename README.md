# blade
A runner for your dev env with file watching and support for multiple services in the same repository

## usage
```shell
blade run
# or
blade run service-name-1 service-name-2 service-name-n...
```
This will read configuration from the blade.yaml file in the current working directory.

## configuration
The following configuration structure is required so we know what services you have, how to run them, and what files to watch to reload the application 
```yaml
- name: "service-one"
  watch:
    fs:
      path: "cmd/service-one/"
      ignore:
        - "node_modules"
        - ".*"
        - "**/*_test.go"
  env:
    - name: "VAR_1"
      value: "VAL_1"
    - name: "VAR_2"
      value: "VAL_2"
    - name: "VAR_NO_VALUE"
    - name: "VAR_EMPTY"
    - name: "PATH"
    - name: "PWD"
    - name: "GOCACHE"
    - name: "HOME"
      value:
  before: echo "do something at first run"
  run: go run cmd/service-one/main.go
- name: "service-two"
  inheritEnv: true
  env:
    - name: "VAR_3"
      value: "VAL_3"
    - name: "VAR_4"
      value: "VAL_4"
    - name: "VAR_NO_VALUE"
    - name: "VAR_EMPTY_QUOTE"
      value: ""
  watch:
    fs:
      paths:
        - "cmd/service-two/main.go"
  run: go run cmd/service-two/main.go
```