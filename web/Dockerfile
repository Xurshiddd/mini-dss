FROM node:22-alpine AS build

WORKDIR /app

COPY package.json ./
RUN npm install --no-audit --no-fund --legacy-peer-deps

COPY . .

# API manzili build paytida bog'lanadi — brauzerdan ochiladigan manzil.
ARG NUXT_PUBLIC_API_BASE=http://localhost:6081
ENV NUXT_PUBLIC_API_BASE=${NUXT_PUBLIC_API_BASE}
RUN npm run build


FROM node:22-alpine

WORKDIR /app
# ⚠️ node:alpine'da tayyor `node` foydalanuvchisi (uid 1000) bor — root
# o'rniga shuni ishlatamiz. SPA statik, yozish huquqi kerak emas.
COPY --from=build --chown=node:node /app/.output ./.output
USER node

ENV PORT=3000
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
