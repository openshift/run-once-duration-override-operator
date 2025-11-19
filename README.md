# README

## FBC catalog rendering

To initiliaze catalog-template.json

```sh
$ opm migrate registry.redhat.io/redhat/redhat-operator-index:v4.17 ./catalog-migrate
$ mkdir -p v4.17/catalog/run-once-duration-override-operator
$ opm alpha convert-template basic ./catalog-migrate/run-once-duration-override-operator/catalog.json > v4.17/catalog-template.json
```

To update the catalog

```
$ cd v4.17
$ export REGISTRY_AUTH_FILE=...
$ opm alpha render-template basic catalog-template.json --migrate-level bundle-object-to-csv-metadata > catalog/run-once-duration-override-operator/catalog.json
```

## Releases

| rodoo version | bundle image                                                     |
| ------------- | ---------------------------------------------------------------- |
| 1.1.0         | 0cf817432277977ae75e28b95be321ff92600fb57f89b54533272657430d16f2 |
| 1.1.1         | ec2ad9acac7336403094c3387975d653a23abf203ba0d5dae0338d62e55f407b |
| 1.1.2         | 7585c6148b774e3a7ae78bba25feb7739f9f64dbfd4de9e3e8776184f48a2764 |
| 1.1.3         | 9d9d030a074de3bfda4ebcf1cb6771f58f33fd77e12dd8a6b59d223bb1735599 |
| 1.2.0         | 68178c1bdb8ea36faf602d639af290096b40d796aaf8f0e66bff1f6de1ec036a |
| 1.2.1         | 75066a651c950ae906bf44fd69391965719556a9640c0cba9f0d3c7da65f8fa5 |
| 1.2.2         | 8ff7f1c707b40448a719137f37f6a3e09860812f7f07c3e61b7f73d1216399dc |
| 1.2.3         | ad4a176cbd498ab2785ccf99a4081ae74c1a0f69dc144d6975d751037f5aa6a6 |
