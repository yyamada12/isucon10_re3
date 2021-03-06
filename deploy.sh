#!/bin/bash

set -eux

date -R
echo "Started deploying."

# git pull
cd ~
git pull origin main

# rotate logs
function rotate_log () {
  if sudo [ -e $1 ]; then
    sudo mv $1 ${1%.*}_$(date +"%Y%m%d%H%M%S").${1##*.}
  fi
}
rotate_log /var/log/nginx/access.log
rotate_log /var/log/mysql/slow.log
rotate_log ~/pprof/pprof.png


# build go app
cd /home/isucon/isuumo/webapp/go
make

# update mysqld.cnf
if [ -e ~/etc/mysqld.cnf ]; then
  sudo cp ~/etc/mysqld.cnf /etc/mysql/mysql.conf.d/mysqld.cnf
fi

# restart services
sudo systemctl restart mysql
sudo systemctl restart isuumo.go
sudo systemctl restart nginx

date -R
echo "Finished deploying."
