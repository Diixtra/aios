from fastapi import HTTPException, Security
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

security = HTTPBearer(auto_error=False)


def require_api_key(expected_key: str):
    def _verify(
        credentials: HTTPAuthorizationCredentials | None = Security(security),
    ):
        if credentials is None or credentials.credentials != expected_key:
            raise HTTPException(status_code=401, detail="Unauthorized")

    return _verify
