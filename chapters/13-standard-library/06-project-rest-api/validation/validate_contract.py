"""Validate the OpenAPI document and live HTTP fixtures; run from any directory.

This optional developer check needs requirements.txt. The Go application itself
uses only the standard library. The check builds one temporary binary, owns its
server process, and stops it even when an assertion fails.
"""

import json
from pathlib import Path
import selectors
import subprocess
import tempfile
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import ProxyHandler, Request, build_opener

from jsonschema import Draft202012Validator
from openapi_spec_validator import validate
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012


PROJECT = Path(__file__).resolve().parents[1]
ROOT = PROJECT.parents[2]
SPEC = json.loads((PROJECT / "httpapi/openapi.json").read_text())
SPEC_URI = "urn:golang-course:contacts-openapi"
REGISTRY = Registry().with_resource(
    SPEC_URI, Resource.from_contents(SPEC, default_specification=DRAFT202012)
)


def resolve(value):
    while "$ref" in value:
        reference = value["$ref"]
        if not reference.startswith("#/"):
            raise AssertionError(f"unexpected external reference: {reference}")
        value = SPEC
        for part in reference[2:].split("/"):
            value = value[part.replace("~1", "/").replace("~0", "~")]
    return value


def validate_value(schema, value):
    # The schema is a subschema of this document, so anchor local refs there.
    wrapper = {"$id": SPEC_URI, **SPEC, "allOf": [schema]}
    Draft202012Validator(wrapper, registry=REGISTRY).validate(value)


def exercise(base_url):
    opener = build_opener(ProxyHandler({}))
    checked = 0

    def call(method, path, expected, body=None, headers=None):
        nonlocal checked
        headers = dict(headers or {})
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            headers.setdefault(
                "Content-Type",
                "application/merge-patch+json" if method == "PATCH" else "application/json",
            )
        request = Request(base_url + path, data=data, method=method, headers=headers)
        if body is not None and expected < 300:
            route = urlsplit(path).path
            if route.startswith("/contacts/"):
                route = "/contacts/{id}"
            operation = SPEC["paths"][route][method.lower()]
            media_type = headers["Content-Type"]
            validate_value(operation["requestBody"]["content"][media_type]["schema"], body)
        try:
            response = opener.open(request, timeout=5)
        except HTTPError as error:
            response = error  # 304 and errors still have headers and an owned body.
        with response:
            status, response_headers = response.status, response.headers
            raw = response.read(512 * 1024 + 1)
        if len(raw) > 512 * 1024:
            raise AssertionError("fixture response exceeded the check's read bound")
        assert status == expected, (method, path, status, raw)
        route = urlsplit(path).path
        if route.startswith("/contacts/"):
            route = "/contacts/{id}"
        operation = SPEC["paths"].get(route, {}).get(method.lower())
        if operation is None:
            key = "x-method-not-allowed-response" if route in SPEC["paths"] else "x-unknown-route-response"
            contract = resolve(SPEC[key])
        else:
            contract = resolve(operation["responses"][str(status)])
        for name, declaration in contract.get("headers", {}).items():
            declaration = resolve(declaration)
            value = response_headers.get(name)
            if declaration.get("required"):
                assert value is not None, (method, path, status, "missing header", name)
            if value is not None:
                validate_value(declaration["schema"], value)
        if method == "HEAD" or status in (204, 304):
            assert raw == b"", (method, path, status, "unexpected body")
            if status in (204, 304):
                assert response_headers.get("Content-Type") is None
            decoded = None
        else:
            media_type = response_headers.get_content_type()
            declaration = contract.get("content", {}).get(media_type)
            assert declaration is not None, (method, path, status, "undocumented media type", media_type)
            decoded = json.loads(raw)
            validate_value(declaration["schema"], decoded)
            if media_type == "application/problem+json":
                assert decoded["status"] == status
        checked += 1
        return response_headers, decoded

    call("GET", "/", 200)
    _, served_spec = call("GET", "/openapi.json", 200)
    assert served_spec == SPEC
    spec_headers, _ = call("HEAD", "/openapi.json", 200)
    call("GET", "/openapi.json", 304, headers={"If-None-Match": spec_headers["ETag"]})
    call("POST", "/openapi.json", 405)
    call("GET", "/contacts", 200)
    created, _ = call("POST", "/contacts", 201, {"name": " Ana ", "email": "ana@example.com"})
    path, tag = created["Location"], created["ETag"]
    call("GET", path, 200)
    call("HEAD", path, 200)
    call("GET", path, 304, headers={"If-None-Match": tag})
    call("HEAD", path, 304, headers={"If-None-Match": "W/" + tag})
    call("OPTIONS", path, 204)
    call("POST", path, 405)
    call("POST", "/contacts", 409, {"name": "Ana"})
    call("POST", "/contacts", 400, {"name": " "})
    call("POST", "/contacts", 400, {"name": "Ignored", "id": 7})
    call("POST", "/contacts", 415, {"name": "Ignored"}, {"Content-Type": "text/plain"})
    call("POST", "/contacts", 413, {"name": "x" * 4096})
    call("PATCH", path, 415, {}, {"Content-Type": "application/json"})
    call("GET", "/contacts?limit=0", 400)
    call("HEAD", "/contacts?limit=0", 400)
    call("GET", "/contacts?limit=1&limit=2", 400)
    call("GET", "/contacts?name_contains=%ZZ", 400)
    call("POST", "/contacts", 201, {"name": "Ana Torres"})
    page_headers, page = call("GET", "/contacts?limit=1&name_contains=Ana&sort=name", 200)
    assert page["pagination"]["total"] == 2
    call("GET", page["links"]["next"], 200)
    call("GET", page["links"]["self"], 304, headers={"If-None-Match": page_headers["ETag"]})
    updated, _ = call("PATCH", path, 200, {"phone": "555-0100"}, {"If-Match": tag})
    call("PATCH", path, 412, {"email": "stale@example.com"}, {"If-Match": tag})
    call("DELETE", path, 412, headers={"If-None-Match": "*"})
    put, _ = call("PUT", path, 200, {"name": " Ana Updated "}, {"If-Match": updated["ETag"]})
    assert put.get("ETag") is None
    fresh, _ = call("GET", path, 200)
    call("PATCH", path, 400, {"name": None, "email": "rejected@example.com"})
    call("DELETE", path, 204, headers={"If-Match": fresh["ETag"]})
    call("GET", path, 404, headers={"If-None-Match": "*"})
    call("HEAD", path, 404)
    call("GET", "/missing", 404)
    return checked


def main():
    validate(SPEC)
    with tempfile.TemporaryDirectory(prefix="contacts-contract-") as directory:
        binary = Path(directory) / "contacts-api"
        subprocess.run(
            ["go", "build", "-o", str(binary), "./chapters/13-standard-library/06-project-rest-api"],
            cwd=ROOT, check=True, timeout=60,
        )
        with tempfile.TemporaryFile(mode="w+") as output:
            server = subprocess.Popen(
                [str(binary), "-serve", "-addr", "127.0.0.1:0"], cwd=directory,
                stdout=subprocess.PIPE, stderr=output, text=True,
            )
            try:
                # A watchdog prevents a startup regression from blocking readline.
                with selectors.DefaultSelector() as selector:
                    selector.register(server.stdout, selectors.EVENT_READ)
                    assert selector.select(timeout=5), "server did not announce startup"
                announcement = server.stdout.readline().strip()
                assert announcement.startswith("Contacts API: http://127.0.0.1:"), announcement
                checked = exercise(announcement.split("Contacts API: ", 1)[1])
            finally:
                server.terminate()
                try:
                    server.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.communicate()
                if server.returncode != 0:
                    output.seek(0)
                    raise AssertionError(f"server did not shut down cleanly: {server.returncode}\n{output.read()}")
    print(f"OpenAPI 3.1.1 valid; {checked} live HTTP responses match documented bodies and headers.")


if __name__ == "__main__":
    main()
