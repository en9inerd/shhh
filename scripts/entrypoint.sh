#!/bin/sh
set -e

export NGINX_CORS_ORIGIN="${NGINX_CORS_ORIGIN:-*}"

# When SSL is enabled, inject an HTTP-to-HTTPS redirect into the port-80 server block
if [ "${NGINX_SSL_ENABLED}" = "true" ]; then
  export NGINX_HTTP_REDIRECT='return 301 https://$host$request_uri;'
else
  export NGINX_HTTP_REDIRECT='# SSL redirect disabled'
fi

envsubst '${NGINX_BACKEND} ${NGINX_SERVER_NAME} ${NGINX_CORS_ORIGIN} ${NGINX_HTTP_REDIRECT}' \
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
