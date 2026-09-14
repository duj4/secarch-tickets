from redis.sentinel import Sentinel

SENTINELS = [
    ("vshgms641.linux.ms.com.cn", 26379),
    ("vshgms266.linux.ms.com.cn", 26379),
    ("vshgms628.linux.ms.com.cn", 26379),
]

CA = "/path/to/ca.pem"
CLIENT_CERT = "/path/to/client.crt"
CLIENT_KEY = "/path/to/client.key"

sentinel = Sentinel(
    SENTINELS,

    socket_timeout=2,
    socket_connect_timeout=2,

    # App -> Sentinel
    sentinel_kwargs={
        "username": "sentinel-client",
        "password": "<SENTINEL_CLIENT_PASSWORD>",

        "ssl": True,
        "ssl_ca_certs": CA,
        "ssl_certfile": CLIENT_CERT,
        "ssl_keyfile": CLIENT_KEY,
        "ssl_cert_reqs": "required",
        "ssl_check_hostname": True,
    },

    # App -> Redis Primary
    username="<APP_REDIS_USERNAME>",
    password="<APP_REDIS_PASSWORD>",

    ssl=True,
    ssl_ca_certs=CA,
    ssl_certfile=CLIENT_CERT,
    ssl_keyfile=CLIENT_KEY,
    ssl_cert_reqs="required",
    ssl_check_hostname=True,
)

print("Master:", sentinel.discover_master("redis-ai-qa"))
print("Replicas:", sentinel.discover_slaves("redis-ai-qa"))

redis_client = sentinel.master_for(
    "redis-ai-qa",
    socket_timeout=2,
)

redis_client.set("sentinel-test", "hello")
print(redis_client.get("sentinel-test"))
