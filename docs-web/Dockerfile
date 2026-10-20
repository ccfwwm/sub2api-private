FROM node:20-alpine

WORKDIR /app
COPY docs-web/server.mjs /app/server.mjs
COPY docs-web/index.html /app/index.html
COPY docs-web/app.mjs docs-web/styles.css /app/
COPY docs/multimodal/imagevideo-content.json /app/content.json
COPY frontend/public/examples/imagevideo.py frontend/public/examples/imagevideo.mjs /app/examples/

EXPOSE 8080

CMD ["node", "/app/server.mjs"]
