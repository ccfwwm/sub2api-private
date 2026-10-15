FROM node:20-alpine

WORKDIR /app
COPY docs-web/server.mjs /app/server.mjs
COPY docs-web/index.html /app/index.html

EXPOSE 8080

CMD ["node", "/app/server.mjs"]
