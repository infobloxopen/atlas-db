# atlas-db-controller

This controller manages three types of resources:
	- Database Servers
	- Databases
	- Database Schemas

## Database Servers

This resource is used manage the lifecyle of a database server instance. Currently
supported are MySQL, PostgresSQL, and RDS instances.

### MySQL

To create a MySQL database server instance, you specify the mySQL member of the
DatabaseServer resource.

```
apiVersion: atlas.infoblox.com/v1alpha1
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

## Databases

This resource is used to manage the lifecycle of specific databases on a database
server instance.

## Database Schemas

This resource is used to manage the lifecycle of the schemas within a database. It
allows automated migration of database objects such as tables, and triggers, and
manages the versioning an execution of those migrations.
