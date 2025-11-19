# README

## FBC catalog rendering

To initiliaze catalog-template.json

```sh
$ opm migrate registry.redhat.io/redhat/redhat-operator-index:v4.15 ./catalog-migrate
$ mkdir -p v4.15/catalog/run-once-duration-override-operator
$ opm alpha convert-template basic ./catalog-migrate/run-once-duration-override-operator/catalog.json > v4.15/catalog-template.json
```

To update the catalog

```
$ cd v4.15
$ opm alpha render-template basic catalog-template.json > catalog/run-once-duration-override-operator/catalog.json
```

## Releases

| rodoo version | bundle image                                                     |
| ------------- | ---------------------------------------------------------------- |
| 1.0.0         | 49f474250adfc7057dc84de25d50ecf6b990f3dcf9874eddfd42477493d50e6c |
| 1.0.1         | 5e2f382d233fab6817da02d17459b3e6e8c16f0be58270221b66d87ce3d09cc6 |
| 1.0.2         | e07406af6f08e311925f92bb4d68e8f5266dba1bb80b5a4fc108c8948644efef |
| 1.0.3         | 88713351f35edfcc8ea38e7d9b1dad293e6105fc30a60865ea49126912fe8318 |
| 1.1.0         | 0cf817432277977ae75e28b95be321ff92600fb57f89b54533272657430d16f2 |
| 1.1.1         | ec2ad9acac7336403094c3387975d653a23abf203ba0d5dae0338d62e55f407b |
| 1.1.2         | 7585c6148b774e3a7ae78bba25feb7739f9f64dbfd4de9e3e8776184f48a2764 |
| 1.1.3         | 9d9d030a074de3bfda4ebcf1cb6771f58f33fd77e12dd8a6b59d223bb1735599 |
