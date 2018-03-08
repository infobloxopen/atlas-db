# atlas-db-controller

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
supported are MySQL, PostgresSQL, and RDS instances.

### Pod-Based Servers

You can create servers that are backed by pods on the local cluster. Note that these are not
production-class deployments, and are meant for development.

These will result in the creation of three resources:

- A `Pod` with the same name as the `DatabaseServer` resource.
- A cluster IP `Service` with the same name as the `DatabaseServer` resource.
- A `Secret` with the DSN to connect as super-user to the database.

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
  port: 3306
  rootPassword: "root"
  mySQL:
    image: mysql
```

### Cloud Servers

## Databases

This resource is used to manage the lifecycle of specific databases on a database
server instance.

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
  mysql:
    dsnFrom:
      secretKeyRef:
        name: mydbserver
        key: dsn
```

## Database Schemas

This resource is used to manage the lifecycle of the schemas within a database. It
allows automated migration of database objects such as tables, and triggers, and
manages the versioning an execution of those migrations.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseSchema
metadata:
  name: mydb
spec:
  database:
    name: mydb
    dsn: dsn_mydbadmin
  schema:
    repo: github.com/infobloxopen/atlas-contacts-app
    dir: migrations
    version: 100
```

Alternatively, if you have manually created the database, you can
explicitly list the database type and connection details.

```
apiVersion: atlasdb.infoblox.com/v1alpha1
kind: DatabaseSchema
metadata:
  name: mydb
spec:
  mysql:
    dsnFrom:
      secretKeyRef:
        name: mydbcreds
        key: dsn
  schema:
    repo: github.com/infobloxopen/atlas-contacts-app
    dir: migrations
    version: 100
```
