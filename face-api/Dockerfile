FROM python:3.12-slim

# insightface onnxruntime uchun kerak bo'ladigan tizim kutubxonalari
RUN apt-get update && apt-get install -y --no-install-recommends \
        libgl1 \
        libglib2.0-0 \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# ⚠️ Non-root. InsightFace modellarni `$HOME/.insightface` ga yozadi —
# shuning uchun HOME belgilanadi va o'sha katalog app egaligida bo'ladi
# (docker-compose'da volume shu yo'lga ulanadi).
RUN useradd -m -u 10001 app && mkdir -p /home/app/.insightface && chown -R app /home/app/.insightface
ENV HOME=/home/app
USER app

EXPOSE 8000

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
