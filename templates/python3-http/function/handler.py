def handle(event: dict, context: dict):
    """
    Função serverless de exemplo no padrão SwitchFaaS.
    Recebe `event` (body, headers, query, method) e `context` (function_name, request_id).
    """
    body = event.get("body")
    if not isinstance(body, dict):
        body = {}

    name = body.get("name", "SwitchFaaS")

    return {
        "message": f"Olá, {name}! Função executada com sucesso na borda.",
        "function": context.get("function_name"),
        "request_id": context.get("request_id"),
        "received_body": body,
    }, 200
