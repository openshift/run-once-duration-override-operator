# README

## FBC catalog rendering

To initiliaze the latest catalog-template.yaml

```sh
$ opm migrate registry.redhat.io/redhat/redhat-operator-index:v4.21 ./catalog-migrate
$ mkdir -p v4.21/catalog/run-once-duration-override-operator
$ opm alpha convert-template basic -o yaml ./catalog-migrate/run-once-duration-override-operator/catalog.json > v4.21/catalog-template.yaml
```

To update the latest catalog

```
$ export REGISTRY_AUTH_FILE=...
$ opm alpha render-template basic v4.21/catalog-template.yaml --migrate-level bundle-object-to-csv-metadata > v4.21/catalog/run-once-duration-override-operator/catalog.json
```
