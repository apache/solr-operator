# Running tests in IDEA

The Solr Operator tests cannot be run by default in IntelliJ or GoLand,
however the project is setup to make this as easy as possible for developers to enable it.

Steps:
1. Install the [GinkGo IDEA Plugin](https://plugins.jetbrains.com/plugin/17554-ginkgo)
2. Run `make idea`

At this point you should be able to run individual tests via IntelliJ/GoLand.
If you need to reset these values, you could close the project, delete the `.idea` folder, then open the project again.
IntelliJ/GoLand should then load in the Testing configuration saved under `.run/`.

The Testing Configuration is saved in the repo under `.run/`.
This is not specific to any installation, so please do not commit any changes to these files.
