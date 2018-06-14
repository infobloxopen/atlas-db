# atlas-db-controller

**only validated with PostgreSQL others are under development**

This controller manages three types of resources:

  - Database Servers
  - Databases
  - Database Schemas

The `DatabaseServer` resource will instantiate a database server and create a `Secret`
that contains the DSN to connect to the database as the super user. This secret is
then used by the `Database` resource, which will create a specific application database
on that server, along with users and associated `Secrets` as defined in the spec. At least
one user with administrative access to that database is required. Finally, the `DatabaseSchema`
resource will use the administrative `Secret` created by the `Database` resource to install
and maintain a specific version of the database schema.

## Database Servers

This resource is used manage the lifecyle of a database server instance. Currently
supported are PostgresSQL & MySQL. RDS instances is under development.

### Pod-Based Servers

You can create servers that are backed by pods on the local cluster. Note that these are not
production-class deployments, and are meant for development.

These will result in the creation of three resources:

- A `Pod` with the same name as the `DatabaseServer` resource.
- A cluster IP `Service` with the same name as the `DatabaseServer` resource.
- A `Secret` with the DSN to connect as super-user to the database.


#### PostgreSQL

To create a PostgreSQL database server instance, you specify the PostgreSQL member of the
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


#### MySQL

To create a MySQL database server instance, you specify the mySQL member of the
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

### Cloud Servers ( WIP )


## Databases

This resource is used to manage the lifecycle of specific databases on a database
server instance. create user specific to this database.

These will create a `Secret` with the DSN to connect as admin-user to the database. One can
skip this by not providing users information. That implies `Database Schema` will use `dsn`
to connect database.


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
  serverType: postgres
```

Or, if you didn't use a `DatabaseServer` to provision the server:

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
  dsnFrom:
    secretKeyRef:
      name: mysecrets
      key: dsn
  serverType: postgres
```

In development environment `dsnFrom` can be replaced with just `dsn`

```
dsn: postgres://postgres:postgres@localhost:5432/postgres
```

## Database Schemas

This resource is used to manage the lifecycle of the schemas within a database. It
allows automated migration of database objects such as tables, and triggers, and
manages the versioning an execution of those migrations.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseSchema
metadata:
  name: myschema
spec:
  database: mydb
  git: github://iburak-infoblox:<place password or oauth token here>@iburak-infoblox/atlas-contacts-app/migrations
  version: 001
```

Alternatively, if you have manually created the database, you can
explicitly list the database type and connection details.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseSchema
metadata:
  name: myschema
spec:
  dsnFrom:
    secretKeyRef:
      name: mydbcreds
      key: dsn
  git: github://iburak-infoblox:<place password or oauth token here>@iburak-infoblox/atlas-contacts-app/migrations
  version: 001
```
In development environment `dsnFrom` can be replaced with just `dsn`

```
dsn: postgres://postgres:postgres@localhost:5432/postgres
```

## Steps to create secrets

Assume 'infoblox@123' is your password for admin user first base64 encode it.

```
$ echo -n "infoblox@123" | tr -d '\n' | base64
aW5mb2Jsb3hAMTIz
```

You have to update this base64 encoded value below
```
---
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

Onemore way to create secret is update the value in a file say `/tmp/dsn`
```
$cat /tmp/dsn
postgres://postgres:postgres@192.168.39.216:5432/postgres?sslmode=disable"
```

Execute below command to create secret name `mydbsecret` with key `dsn`
in default namespace.
```
kubectl create secret -n default generic mydbsecret --from-file=/tmp/dsn
```
