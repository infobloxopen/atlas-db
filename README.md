[![Build Status](https://travis-ci.org/infobloxopen/atlas-db.svg?branch=master)](https://travis-ci.org/infobloxopen/atlas-db)

# atlas-db-controller

**Current support only for Postgres database; other databases are under development.**

Atlas-db-controller is used to create custom resource and manage them.

It manages 3 types of resources:
  - Database Servers
  - Databases
  - Database Schemas

The `DatabaseServer` resource will instantiate a database server and create a `Secret`
that contains the DSN to connect to the database instance as the super user.

This secret is then used by the `Database` resource, which will create specific
application database on that server, along with users and associated `Secrets` as defined
in the spec. **At least one user with administrative access to the database is required.**

`DatabaseSchema` resource will use the administrative `Secret` created by the `Database`
resource to install and maintain a specific version of the database schema.

## Prerequisite
   - Kubernetes cluster version should be > 1.9
   - Migration/Initialization scripts should comply with [these](https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md) guidelines.
   - To access gitHub source need user's **personal access tokens**
   - `GitHub` and `AWS S3` are the only supported source for migration scripts.
   - Current end to end support for Postgres database only.

## Custom Resources and Controller deployment
Users need to create custom resources and controller instance to manage custom resources in
kubernetes cluster.

##### Custom Resource
To read about Custom resource follow [link](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

To create the custom resources execute:
```
kubectl create -f ./deploy/crd.yaml
```
This will define `DatabaseServers`, `Database` and `DatabaseSchema` custom resource in
kubernetes cluster.

##### Controller

To create the controller:
```
kubectl create -f ./deploy/namespace.yaml
kubectl create -f ./deploy/atlas-db.yaml
```
*If user wishes to deploy controller in a different namespace; edit namespace.yaml and
atlas-db.yaml files respectively.*

Alternatively several developer options can be configured in `atlas-db.yaml` to customize
the controller deployment as follow:

  - resync: Resync duration
  - logtostderr: Logs are written to standard error instead of files.
  - log: log level
  - v: Enable debug mode
  - LabelSelector: A selector to restrict the list of returned objects by their labels.
*If user do not specify this option, atlas-db-controller will default to everything.*

```
...
imagePullPolicy: Always
args:
  - "-resync=3m"
  - "-logtostderr"
  - "-v=4"
  - "-log=error"
  - "-l=monitor=atlas-deployment-1"
```

For LabelSelector option to work as intended, resources should have proper labelling.
```
<!--
dbserverA.yaml
-->

...
kind: DatabaseServer
metadata:
  name: postgres
  labels:
    monitor: atlas-deployment-1
  spec:
...
```


## atlas-db resources and deployments

Following section describes the custom resources created and managed by atlas-db-controller.

### Database Servers

Database Server resource is used manage the lifecycle of a database server instance. *RDS DB servers are planned to provision manually.*

#### Pod-Based Servers

User can create servers that are backed by pods on the local cluster. *Note that these are not
production-class deployments, and are meant for development purposes only.*

These will result in the creation of 3 resources:

- A `Pod` with the same name as the `DatabaseServer` resource.
- A cluster IP `Service` with the same name as the `DatabaseServer` resource.
- A `Secret` with the DSN to connect as super-user to the database.


##### PostgreSQL

To create a Postgres database server instance, user specify the Postgres member of the
DatabaseServer resource.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseServer
metadata:
  name: mydbserver
spec:
  servicePort: 5432
  superUser: "postgres"
  superUserPassword: "postgres"
  postgres:
    image: postgres
```


##### MySQL

To create a MySQL database server instance, user specify the MySQL member of the
DatabaseServer resource.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseServer
metadata:
  name: mydbserver
spec:
  servicePort: 3306
  rootPassword: "root"
  mySQL:
    image: mysql
```

#### Cloud Servers
**NOTE: RDS database server needs to be provisioned manually and user should get login credentials.**


When using External Database server or RDS database instance user need to create a
database server resource in Kubernetes cluster which will create an external service
referring to External Database server or RDS database instance.

These will result in the creation of 2 resources:

- A Externalname `Service` with the same name as the `DatabaseServer` resource.
- A `Secret` with the DSN to connect as super-user to the database.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseServer
metadata:
  name: mydbserver
spec:
  servicePort: 5432
  superUser: "postgres"
  superUserPassword: "postgres"
  dbHost: "aws.rds.db.instance.infoblox.com"
  rds:
    engine: postgres
```


### Databases

Database resources are used to manage the lifecycle of specific databases on a database
server instance.
This resource will create specified user in the database instance as well. *An user with
admin role is a must*.

This resource will also create a `Secret` with the DSN to connect as admin-user to the
database. *User can skip this by not providing "users" information*. That implies
`DatabaseSchema` will use user provided `dsn` to connect to database resource.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: Database
metadata:
  name: mydb
spec:
  users:
  - name: mydb
    password: foo
    role: read
  - name: mydbadmin
    passwordFrom:
      secretKeyRef:
        name: mydbsecrets
        key: adminpw
  server: mydbserver
```

### Database Schemas

Database Schema resource is used to manage the lifecycle of the schemas within a database.
This allows automated migration of database objects such as tables and triggers; and
manages the execution versioning of those migrations.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseSchema
metadata:
  name: myschema
spec:
  database: mydb
  source: github://iburak-infoblox:<place password or oauth token here>@infobloxopen/atlas-contacts-app/db/migrations
  version: 001
```

User can use `sourceFrom` as an alternate to `source` to secure the credentials.

**NOTE: Need to use different names for resources database server, database and database schema. If same name given for all the resources, will get this error "secret already exists".**

## Additional details

### construct of GitHub url as a Source

`github://user:personal-access-token@owner/repo/path#ref`

| URL Query  | Description |
|------------|-------------|
| user | The username of the user connecting to github |
| personal-access-token  | An access token from Github [link](https://github.com/settings/tokens) |
| owner | The repo owner |
| repo | The name of the git repository |
| path | Path in the git repository to migrations |
| ref | **(optional)** can be a SHA, branch, or tag |

### construct of S3 Url as a Source

`s3://<bucket-name>/<prefix>`

| URL Query | Description |
|-----------|-------------|
| bucket-name | name of the bucket in which migration file lives. |
| prefix | **(optional)** Limits the response to keys that begin with the specified prefix. |


### Steps to create kubernetes secrets to secure (passwords, dsn, sourceURL)

Assume 'infoblox@123' is the password for admin user.

First `base64` encode it.

```
$ echo -n "infoblox@123" | tr -d '\n' | base64
aW5mb2Jsb3hAMTIz
```

User have to update this base64 encoded value below
```
...
apiVersion: v1
kind: Secret
metadata:
  name: mydbsecret
  namespace: default
type: Opaque
data:
  adminUserPass: aW5mb2Jsb3hAMTIz
  dsn: <UPDATE YOUR ENCODED VALUE>
```

Additional way to create secret is to update the value in a file say `/tmp/dsn`
```
$cat /tmp/dsn
postgres://postgres:postgres@192.168.39.216:5432/postgres?sslmode=disable"
```

Then execute the below command to create secret named `mydbsecret` with key `dsn` in
default namespace.
```
kubectl create secret -n default generic mydbsecrets --from-file=/tmp/dsn
```
