import os
import traceback
import uuid
from flask import Flask, request, jsonify
from waitress import serve
from function import handler

app = Flask(__name__)


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"}), 200


@app.route("/", methods=["POST", "GET"])
def invoke():
    # Extrai o corpo tentando fazer parse de JSON; se não for JSON, usa texto bruto
    body = request.get_json(silent=True)
    if body is None and request.data:
        body = request.data.decode("utf-8", errors="replace")

    event = {
        "body": body,
        "headers": dict(request.headers),
        "query": request.args.to_dict(),
        "method": request.method,
    }

    context = {
        "function_name": os.getenv("FUNCTION_NAME", "local-function"),
        "request_id": request.headers.get("X-Request-Id", str(uuid.uuid4())),
    }

    try:
        result = handler.handle(event, context)

        # Permite retornar (dados, status_code) ou apenas dados (padrão 200)
        if isinstance(result, tuple) and len(result) == 2:
            payload, status_code = result
        else:
            payload, status_code = result, 200

        if isinstance(payload, (dict, list)):
            return jsonify(payload), status_code
        return str(payload), status_code

    except Exception as exc:
        traceback.print_exc()
        return jsonify({
            "error": "Erro interno ao executar a função",
            "details": str(exc),
        }), 500


if __name__ == "__main__":
    port = int(os.getenv("PORT", "8080"))
    workers = int(os.getenv("WORKERS", "8"))
    print(f"[Runtime Wrapper] Iniciando servidor na porta {port} (workers={workers})...")
    serve(app, host="0.0.0.0", port=port, threads=workers)
