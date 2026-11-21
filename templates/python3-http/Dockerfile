FROM python:3.12-slim

# Evita geração de arquivos .pyc e força logs em tempo real no stdout do container
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PORT=8080 \
    WORKERS=8

WORKDIR /app

# 1. Instala dependências do Runtime Wrapper primeiro (aproveita cache de camadas do Docker)
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# 2. Instala dependências específicas da função do usuário
COPY function/requirements.txt ./function/requirements.txt
RUN pip install --no-cache-dir -r ./function/requirements.txt

# 3. Copia o servidor Wrapper e o código da função
COPY index.py ./
COPY function/ ./function/

EXPOSE 8080

CMD ["python", "index.py"]
