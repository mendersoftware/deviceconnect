FROM python:3.9-alpine3.12

COPY . /testing

RUN pip3 install --quiet -r /testing/requirements.txt

ENTRYPOINT ["/bin/sh", "/testing/entrypoint.sh"]
