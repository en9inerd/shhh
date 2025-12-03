#!/bin/sh
set -e

export NGINX_CORS_ORIGIN="${NGINX_CORS_ORIGIN:-*}"

envsubst '${NGINX_BACKEND} ${NGINX_SERVER_NAME} ${NGINX_CORS_ORIGIN}' \
  < /etc/nginx/nginx.conf.template \
  > /etc/nginx/nginx.conf

# Process SSL server block config only if SSL is enabled
if [ "${NGINX_SSL_ENABLED}" = "true" ]; then
  envsubst '${NGINX_BACKEND} ${NGINX_SERVER_NAME} ${NGINX_CORS_ORIGIN}' \
    < /etc/nginx/nginx-ssl.conf.template \
    > /etc/nginx/ssl-server.conf
else
  echo "" > /etc/nginx/ssl-server.conf
fi

nginx -t
nginx

exec su-exec app /app/shhh
