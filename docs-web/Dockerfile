FROM nginx:1.27-alpine

COPY docs-web/nginx.conf /etc/nginx/conf.d/default.conf
COPY docs-web/index.html /usr/share/nginx/html/index.html

EXPOSE 8080
