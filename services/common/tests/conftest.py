import urllib.request

import pytest


class _DummyVictoriaResponse:
    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False

    def read(self):
        return b"ok"


@pytest.fixture(autouse=True)
def _mock_victorialogs(monkeypatch):
    monkeypatch.setattr(urllib.request, "urlopen", lambda *args, **kwargs: _DummyVictoriaResponse())
