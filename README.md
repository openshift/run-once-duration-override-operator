# README

## FBC catalog rendering

To initiliaze catalog-template.json

```sh
$ opm migrate registry.redhat.io/redhat/redhat-operator-index:v4.14 ./catalog-migrate
$ mkdir -p v4.14/catalog/run-once-duration-override-operator
$ opm alpha convert-template basic ./catalog-migrate/run-once-duration-override-operator/catalog.json > v4.14/catalog-template.json
```

To update the catalog

```
$ cd v4.14
$ opm alpha render-template basic catalog-template.json > catalog/run-once-duration-override-operator/catalog.json
```

## Releases

| rodoo version | bundle image                                                     |
| ------------- | ---------------------------------------------------------------- |
| 1.0.0         | 49f474250adfc7057dc84de25d50ecf6b990f3dcf9874eddfd42477493d50e6c |
| 1.0.1         | 5e2f382d233fab6817da02d17459b3e6e8c16f0be58270221b66d87ce3d09cc6 |
| 1.0.2         | e07406af6f08e311925f92bb4d68e8f5266dba1bb80b5a4fc108c8948644efef |
| 1.0.3         | 88713351f35edfcc8ea38e7d9b1dad293e6105fc30a60865ea49126912fe8318 |
