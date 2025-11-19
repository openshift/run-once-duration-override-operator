# README

## FBC catalog rendering

To initiliaze catalog-template.json

```sh
$ opm migrate registry.redhat.io/redhat/redhat-operator-index:v4.18 ./catalog-migrate
$ mkdir -p v4.19/catalog/run-once-duration-override-operator
$ opm alpha convert-template basic ./catalog-migrate/run-once-duration-override-operator/catalog.json > v4.19/catalog-template.json
```

To update the catalog

```
$ cd v4.19
$ export REGISTRY_AUTH_FILE=...
$ opm alpha render-template basic catalog-template.json --migrate-level bundle-object-to-csv-metadata > catalog/run-once-duration-override-operator/catalog.json
```

## Releases

| rodoo version | bundle image                                                     |
| ------------- | ---------------------------------------------------------------- |
| 1.2.0         | 68178c1bdb8ea36faf602d639af290096b40d796aaf8f0e66bff1f6de1ec036a |
| 1.2.1         | 75066a651c950ae906bf44fd69391965719556a9640c0cba9f0d3c7da65f8fa5 |
| 1.2.2         | 8ff7f1c707b40448a719137f37f6a3e09860812f7f07c3e61b7f73d1216399dc |
| 1.2.3         | ad4a176cbd498ab2785ccf99a4081ae74c1a0f69dc144d6975d751037f5aa6a6 |
| 1.3.0         | 1fd96befffa2475efa28bf83a7beff4f1df5343644b8a203feb44ff03657280b |
| 1.3.1         | 783c2df43ece311bcbaaf30f33c081608e996a0f651895ec0509bb73a6a1f590 |
