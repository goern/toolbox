FROM registry.fedoraproject.org/fedora:32

ENV NAME=tensorflow-jupyter-toolbox VERSION=v0.1.0
LABEL com.github.containers.toolbox="true" \
      com.github.debarshiray.toolbox="true" \
      com.redhat.component="$NAME" \
      name="$FGC/$NAME" \
      version="$VERSION" \
      usage="This image is meant to be used with the toolbox command" \
      summary="Base image for ..." \
      maintainer="Red Hat's AI CoE <aicoe-thoth@redhat.com>"

COPY README.md /

RUN sed -i '/tsflags=nodocs/d' /etc/dnf/dnf.conf

COPY missing-docs /
RUN dnf -y reinstall $(<missing-docs)
RUN rm /missing-docs

COPY extra-packages /
RUN dnf -y install $(<extra-packages)
RUN rm /extra-packages

RUN dnf clean all

COPY Pipfile /
RUN pipenv --python /usr/bin/python3.6 install

CMD /bin/sh
