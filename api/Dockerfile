FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api


FROM alpine:3.21

# tzdata — qurilma vaqti local (UTC+5), farqni hisoblash uchun zona kerak.
# ca-certificates — HEMIS rasmlarini HTTPS orqali yuklash uchun.
RUN apk add --no-cache tzdata ca-certificates

COPY --from=build /out/api /usr/local/bin/api

# ⚠️ Non-root foydalanuvchi: konteyner buzilsa ham hujum yuzasi torroq.
# Port 8000 (>1024), shuning uchun root shart emas. Rasm katalogi shu
# foydalanuvchi egaligida.
RUN adduser -D -u 10001 app && mkdir -p /data/photos && chown app /data/photos
USER app

EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/api"]
