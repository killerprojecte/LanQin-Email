FROM node:20-bookworm-slim AS build
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/package-lock.json* ./
RUN npm ci || npm install
COPY apps/web ./
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY deploy/nginx/web.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
