# How to use example files in this directory

Request for creation of resources (Database Servers, Databases, Database Schemas) to
DB controller can be performed on different ways along with other dependent K8s resources
like pod, service, secrets.



## Single file for all resources & plain text creds
### postgres.yaml

This file create DB server, DB & DB schema resources, secrets, postgresql pod &
service. *All passwords are provided in plain text.*

  - Database Servers + DB server secret ( DSN with superUser )
  - Databases + DB secret ( DSN with admin User )
  - Database Schemas


## Single file for all resources & creds from secrets
### postgres_secrets.yaml

This file create DB server, DB & DB schema resources, secrets, postgresql pod &
service. *All passwords are provided using k8s secret.* Refer main README.md to
create secrets.

  - Database Servers + DB server secret ( DSN with superUser )
  - Databases + DB secret ( DSN with admin User )
  - Database Schemas

## Individual files for each resource
### databaseA.yaml

This file just create DB server resource plus secret, postgresql pod & service

  - Database Servers + DB server secret ( DSN with superUser )

### dbserverA.yaml

This file just create DB resource and will use dsn provided to connect to the 
database server. It will not dependent on DB server resource and will not create
admin user DSN secret because no users are porvided. 
  
  - Databases

### schemaA.yaml

This file just create DB schema resource and will use dsn provided to connect to the
database server. It will not dependent on DB resource. 

  - Database Schemas ( from master branch of the repo )



## mysql.yaml

WIP
