FROM centos:7.4.1708

RUN curl -o /etc/yum.repos.d/CentOS-Base.repo http://mirrors.aliyun.com/repo/Centos-7.repo && \
    yum makecache && \
    yum install -y libaio numactl && \
    yum clean all && \
    rm -rf /var/cache && rm -rf /etc/yum.repos.d/*

RUN groupadd greatdb --gid 1000 && \
    useradd -r -g greatdb -s /bin/false --uid 1000 greatdb && \
    mkdir -p /greatdb /backup   /home/greatdb && \
    chown -R greatdb:greatdb /opt /greatdb /backup  /home/greatdb

COPY  --chown=greatdb:greatdb mysql /opt/greatdb
COPY --chown=greatdb:greatdb xtrabackup /opt/xtrabackup
COPY --chown=greatdb:greatdb bin /usr/local/bin/
COPY --chown=greatdb:greatdb greatdb.cnf /etc/greatdb/greatdb.cnf
    
ENV PATH="${PATH}:/opt/greatdb/bin:/opt/xtrabackup/bin"

USER greatdb
ENTRYPOINT ["start-greatdb.sh"]