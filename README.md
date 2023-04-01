# MySQL-master-slave-docker
##This step, we want to running MySQL on same host docker as master slave server. For this matter, we had this architecture:

If you want to running this project, at the first, you need to docker. So please install docker by this reference. And then you must passing the following step :
1-check the docker were started:
	Systemctl status docker
2-pulling mysql latest image from docker hub:
	Docker image pull mysql
3-create directories for this project:
	#this directory is for store master data file and will mounted on /var/lib/mysql
	Mkdir -p  ~/master-slave/master/db_master	
	#this directory is for store configuration of mysql master
	Mkdir -p ~/master-slave/master/config
	#this directory is for store slave data files and will mounted on /var/lib/mysql
	Mkdir -p ~/master-slave/slave/db_slave
#this directory is for store configuration of slave
	Mkdir -p ~/master-slave/slave/config

4-install necessary packages:
	Yum -y install vim git wget curl yum-utils net-tools tree  telnet && yum -y update
5-create mysql master config file. Please pay attention to this file. This file is very important
	Vim 10-mysqld.cnf
		[mysqld]
		Pid-file=/var/run/mysqld/mysqld-pid
		Socket=/var/run/mysqld/mysqld.sock
		Datadir=/var/lib/mysql	symbolic-link= 0
6-this file must be exist in master and slave config directory on the host machine:	
	Cp -va 10-mysqld.cnf	~/master-slave/master/config
7-now, we are creating a master config file, specific for replication:
	Vim ~/master-slave/master/config/60-enable-replication.cnf
		[mysqld]
		#this is unique for master and slave, master server ids must be greater than slave server id
		Server-id	=	1
		Log-bin		=	mysql-bin
		Binlog_do_db	=	test_db  # this is a name of database which must be replication
8-it is time to running mysql master container:
	Docker run -d –rm –name mysql-master -p 33060:3306 -e MYSQL_ROOT_PASSWORD=123456@Aa -e MYSQL_DATABASE=test_db -v /root/master-slave/master/db_master:/var/lib/mysql -v /root/master-slave/master/config/:/etc/mysql/mysql.conf.d mysql
9-check the container is up and status is normal:
	Docker ps
10- going to the container and adding slave user for replication:
	Docker exec -it mysql-master /bin/bash
		Mysql -uroot -p123456@Aa
			Create user ‘slave’@’%’ identified with mysql_native_password by ‘123456@Aa’;
			Grant replication slave on *.* to ‘slave’@’%’;
			Flush privileges;
11-now you must getting dump mysql master and import to slave:
		Mysqldump -uroot -p123456@Aa test_db>/var/lib/mysql/data.sql
12-and then check the master log file name and position:
	Mysql>Show master status;
13-now we are going to create slave container. For the first, create the slave config file for replication:
	Vim ~/master-slave/slave/config/60-enable-replication.cnf
		[mysqld]
		Server-id	=	2
		Relay_log	=	mysql-relay
		Log_bin		=	mysql-bin
		Binlog_do_db	=	test_db
		Read_only	=	1
14- next, you must running container:
	Docker run –rm -d –name mysql-slave –link mysql-master:db -p 33061:3306 -e MYSQL_ROOT_PASSWORD=123456@Aa -e MYSQL_DATABASE=test_db -v /root/master-slave/slave/db_slave/:/var/lib/mysql -v /root/master-slave/slave/config/:/etc/mysql/mysql.conf.d mysql
15- copy dump to path of slave container:
	Cp -va /root/master-slave/master/db_master/data.sql /root/master-slave/slave/db_slave/
16-exec to container to import dump :
	Docker exec -it mysql-slave /bin/bash
		Mysqldump -uroot -p123456@Aa test_db < /var/lib/mysql/data.sql
		Mysql -uroot =p123456@Aa
			Change master to MASTER_HOST=’db’ , MASTER_USER=’slave’ , MASTER_PASSWORD=’123456@Aa’ , MASTER_LOG_FILE=binlog.000002 , MASTER_LOG_POS= 829;
			Set global server_id=2;
			Start slave;
			Show slave status \G;


