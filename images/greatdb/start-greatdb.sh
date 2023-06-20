#!/bin/bash
mysql_client="mysql"
if which greatdb >/dev/null 2>&1; then
    mysql_client="greatdb"
fi
mysql_server=$mysql_client"d"
echo "client is: "$mysql_client "server is: "$mysql_server

pre_check() {
    if [ -z "$DATABASE_DIR" ]; then
        DATABASE_DIR="/greatdb/mysql/"
        echo $DATABASE_DIR
    fi

    if [ -d "$DATABASE_DIR/conf" ]; then
        DATABASE_ALREADY_EXISTS="true"

        if [ -f "/greatdb/mysql/conf/initdata" ]; then
            DATABASE_ALREADY_INIT="true"
        fi

        if [ -f "/greatdb/mysql/conf/init" ]; then
            DATABASE_USER_ALREADY_INIT="true"
        fi
    fi

    if [ -f "$DATABASE_DIR/data/xtrabackup_info" ]; then
        IS_FROM_BACKUP="true"
    fi
}

init_config() {

    cp /etc/greatdb/greatdb.cnf /greatdb/mysql/conf/my.cnf

    # Generate server ID
    if [ -f "/greatdb/mysql/conf/server_id" ]
        then
            server_id=$(cat /greatdb/mysql/conf/server_id)
            if [ -z "$server_id" ]; then
                server_id=$(date '+%s')
                host_id=$(cat /proc/sys/kernel/hostname | awk -F - '{ print $NF}' | grep -o '[0-9]\+' | tr '\n' '\0' )
                server_id=$server_id$host_id
                server_id=${server_id: -6}
                echo $server_id >/greatdb/mysql/conf/server_id
            fi
        else
            server_id=$(date '+%s')
            host_id=$(cat /proc/sys/kernel/hostname | awk -F - '{ print $NF}' | grep -o '[0-9]\+' | tr '\n' '\0' )
            server_id=$server_id$host_id
            server_id=${server_id: -6}
            echo $server_id >/greatdb/mysql/conf/server_id
    fi

    sed -i "s/SERVERID/$server_id/g" /greatdb/mysql/conf/my.cnf
    domain=${FQDN}
    sed -i "s/REPORTHOST/$domain/g" /greatdb/mysql/conf/my.cnf
    sed -i "s/GROUPLOCALADDRESS/$GROUPLOCALADDRESS/g" /greatdb/mysql/conf/my.cnf
    # Output Profile Content
    cat /greatdb/mysql/conf/my.cnf
}

init_database_dir() {

    # if [ ! -d "/var/lib/mysql-files" ]; then
    #     echo "Directory does not exist, create directory /var/lib/mysql-files"
    #     mkdir -p /var/lib/mysql-files
    # fi

    if ! (id -u greatdb); then
        groupadd greatdb
        useradd -r -g greatdb -s /bin/false greatdb
    fi

    if [ "$IS_FROM_BACKUP" == "true" ]; then
        # backup
        backup_start_init_database_dir
        return
    fi

    normal_start_init_database_dir
    if [ "$?" != "0" ]; then
        exit 1
    fi
}

backup_start_init_database_dir() {
    if [ "$DATABASE_ALREADY_EXISTS" != "true" ]; then
        echo "Directory does not exist, create directory"
        mkdir -p /greatdb/mysql/{socket,logfile,pid,tmp,conf}
    fi

    # chown -R greatdb:greatdb /greatdb/mysql/
    # init config
    init_config
    echo 1 >/greatdb/mysql/conf/initdata
}

normal_start_init_database_dir() {
    if [ "$DATABASE_ALREADY_EXISTS" != "true" ]; then
        echo "Directory does not exist, create directory"
        mkdir -p /greatdb/mysql/{socket,data,logfile,pid,tmp,conf}
    fi

    # chown -R greatdb:greatdb /greatdb/mysql/
    # init config
    init_config

    if [ "$DATABASE_ALREADY_INIT" != "true" ]; then
        echo "Start Initializing Database"
        rm -rf /greatdb/mysql/data/*
        $mysql_server --defaults-file=/greatdb/mysql/conf/my.cnf --initialize-insecure

        if [ "$?" -eq "0" ]; then
            echo 1 >/greatdb/mysql/conf/initdata
            echo "Successfully initialized the database"
        else
            echo "Failed to initialize database"
            exit 1
        fi
    fi
}

init_user_sql() {
    until [ -S "/greatdb/mysql/socket/mysql.sock" ]; do
        echo "Wait for MySQL to be ready"
        sleep 5
    done

    if [ "$DATABASE_USER_ALREADY_INIT" == "true" ]; then
        return
    fi
    echo "Initialize db"


    $mysql_client -S /greatdb/mysql/socket/mysql.sock -u root <<-EOSQL

        create user if not exists root@'%' identified with mysql_native_password by '${ROOTPASSWORD}';
        grant all on *.* to root@'%' with grant option;
        alter user 'root'@'localhost' identified with mysql_native_password by '';

        create user if not exists '${ClusterUser}'@'%' identified with mysql_native_password by '${ClusterUserPassword}';
        grant all on *.* to '${ClusterUser}'@'%' with grant option;

        CHANGE MASTER TO MASTER_USER='${ClusterUser}', MASTER_PASSWORD='${ClusterUserPassword}' FOR CHANNEL 'group_replication_recovery';

        flush privileges;

        reset master;
        reset slave;
EOSQL
    if [ "$?" != "0" ]; then
        echo "Failed to initialized user"
        exit 1
    fi

    echo "Successfully initialized user"
    echo 1 >/greatdb/mysql/conf/init

}

init_gtid_sql() {

    until [ -S "/greatdb/mysql/socket/mysql.sock" ]; do
        echo "Wait for MySQL to be ready"
        sleep 5
    done

    if [ "$DATABASE_USER_ALREADY_INIT" == "true" ]; then
        return
    fi
    for val in $(cat /greatdb/mysql/data/xtrabackup_info | grep GTID); do
        gtid_purged=$val
    done

    if [ "$gtid_purged" == "" ]; then
        echo "reset"
        $mysql_client -S /greatdb/mysql/socket/mysql.sock -u"${ClusterUser}" -p"${ClusterUserPassword}" <<-EOSQL
        stop slave;
        reset master;
        reset slave;
EOSQL

    else
        echo "reset and set  gtid_purged"
        $mysql_client -S /greatdb/mysql/socket/mysql.sock -u"${ClusterUser}" -p"${ClusterUserPassword}" <<-EOSQL
        stop slave;
        reset master;
        reset slave;
        set global gtid_purged=${gtid_purged};
EOSQL

    fi

    if [ "$?" != "0" ]; then
        echo "Failed to reset slave"
        exit 1
    fi

    echo "reset Successfully"

    echo 1 >/greatdb/mysql/conf/init
    DATABASE_USER_ALREADY_INIT="true"

}

init_sql(){
     if [ "$IS_FROM_BACKUP" == "true" ]; then

        init_gtid_sql
        if [ "$?" != "0" ]; then
            echo "Failed to start the database"
            exit 1
        fi
        return
    fi

   
    init_user_sql
    if [ "$?" != "0" ]; then
            echo "Failed to start the database"
            exit 1
    fi

}

start_mysql() {
    echo "start "
     rm -rf /greatdb/mysql/socket/*
    $mysql_server --defaults-file=/greatdb/mysql/conf/my.cnf &
}

_main() {
    pre_check
    init_database_dir
    if [ "$?" != "0" ]; then
        echo "Failed to start the database"
        exit 1
    fi
   
    start_mysql
    if [ "$?" != "0" ]; then
        echo "Failed to start the database"
        exit 1
    fi

    init_sql
    if [ "$?" != "0" ]; then
        echo "Failed to exec init sql "
        exit 1
    fi

    echo "start  Successfully"
   
}

_main

wait
