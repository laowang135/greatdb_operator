FROM centos:7.4.1708
RUN curl -o /etc/yum.repos.d/CentOS-Base.repo http://mirrors.aliyun.com/repo/Centos-7.repo && \
    sed -i 's/$releasever/8/g' /etc/yum.repos.d/CentOS-Base.repo && \
     yum makecache && \
    yum install -y socat which sudo openssh-clients libaio numactl && \
    yum clean all && \
    rm -rf /var/cache && rm -rf /etc/yum.repos.d/*
RUN groupadd greatdb && \
    useradd -r -g greatdb -s /bin/false greatdb && \
    mkdir -p /greatdb  && \
    chown -R greatdb:greatdb /opt /greatdb 
    
COPY --chown=greatdb:greatdb greatdb /opt/greatdb/


COPY --chown=greatdb:greatdb bin /usr/local/bin/
COPY --chown=greatdb:greatdb greatdb.cnf /etc/greatdb/greatdb.cnf
    
ENV PATH="${PATH}:/opt/greatdb/bin"

USER greatdb
ENTRYPOINT ["start-greatdb.sh"]