namespace go config

struct GetConfigRequest {
    1: string version
}

struct GetConfigResponse {
    1: bool useRedis
}

service ConfigService {
    GetConfigResponse getConfig(1: GetConfigRequest req)
}