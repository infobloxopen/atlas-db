# How to use example files.

DB controller manages three types of resources:

  - Database Servers
  - Databases
  - Database Schemas

which can be created in different ways along with other dependent K8s resources
like pod, service, secrets.

## postgres.yaml 

This file create DB server, DB & DB schema resources, secrets, postgresql pod &
service. *All passwords are provided in plain text.*

  - Database Servers + DB server secret ( DSN with superUser )
  - Databases + DB secret ( DSN with admin User )
  - Database Schemas

## postgres_secrets.yaml 

This file create DB server, DB & DB schema resources, secrets, postgresql pod &
service. *All passwords are provided using k8s secret.* Refer main README.md to
create secrets.

  - Database Servers + DB server secret ( DSN with superUser )
  - Databases + DB secret ( DSN with admin User )
  - Database Schemas

## databaseA.yaml

This file just create DB server resource plus secret, postgresql pod & service

  - Database Servers + DB server secret ( DSN with superUser )

## dbserverA.yaml

This file just create DB resource and will use dsn provided to connect to the 
database server. It will not dependent on DB server resource and will not create
admin user DSN secret because no users are porvided. 
  
  - Databases

## schemaA.yaml

This file just create DB schema resource and will use dsn provided to connect to the
database server. It will not dependent on DB resource. 

  - Database Schemas ( from master branch of the repo )

## mysql.yaml

WIP
