from pydantic_settings import BaseSettings


class McpSettings(BaseSettings):
    aios_search_url: str
    aios_api_key: str
