#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import sys


# inner 接口：非公开，需要用户认证
INNER_EXTENSIONS = {
    "isPublic": False,
    "allowApplyPermission": True,
    "authConfig": {
        "appVerifiedRequired": True,
        "userVerifiedRequired": True,
        "resourcePermissionRequired": True
    }
}

# 非 inner 接口：公开，免用户认证
PUBLIC_EXTENSIONS = {
    "isPublic": True,
    "allowApplyPermission": True,
    "authConfig": {
        "appVerifiedRequired": True,
        "userVerifiedRequired": False,
        "resourcePermissionRequired": True
    }
}


def inject_bk_gateway_config(file_path):
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            swagger_data = json.load(f)
    except Exception as e:
        print(f"读取或解析文件失败 [{file_path}]: {e}")
        return

    paths = swagger_data.get("paths", {})
    inner_count = 0
    public_count = 0

    for path, path_item in paths.items():
        for method, method_config in path_item.items():
            if not isinstance(method_config, dict):
                continue
            if "/inner/" in path:
                method_config["x-bk-apigateway-resource"] = INNER_EXTENSIONS
                inner_count += 1
            else:
                method_config["x-bk-apigateway-resource"] = PUBLIC_EXTENSIONS
                public_count += 1

    total = inner_count + public_count
    if total > 0:
        try:
            with open(file_path, 'w', encoding='utf-8') as f:
                json.dump(swagger_data, f, indent=2, ensure_ascii=False)
            print(f"成功, 已为 {file_path} 注入蓝鲸网关配置: "
                  f"{inner_count} 个 inner 接口, {public_count} 个公开接口。")
        except Exception as e:
            print(f"写入文件失败 [{file_path}]: {e}")
    else:
        print(f"未在 {file_path} 中检测到接口，跳过注入。")

if __name__ == "__main__":
    # 支持从命令行传入多个文件路径
    if len(sys.argv) < 2:
        print("使用方法: python3 inject_bk_gateway.py <file1.json> <file2.json> ...")
        sys.exit(1)
        
    for target_file in sys.argv[1:]:
        inject_bk_gateway_config(target_file)