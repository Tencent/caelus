FROM centos:7

# install rpm build tools
RUN yum update -y && \
  yum install -y rpm-build make && \
  mkdir -p /root/rpmbuild/SPECS && \
  mkdir -p /root/rpmbuild/SOURCES

# install golang
RUN curl -fsSL https://dl.google.com/go/go1.15.6.linux-amd64.tar.gz | tar -xzC /usr/local
ENV PATH=/usr/local/go/bin:$PATH
