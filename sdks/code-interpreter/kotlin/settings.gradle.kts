/*
 * Copyright 2025 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

rootProject.name = "code-interpreter-parent"

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version("1.0.0")
}

include(":code-interpreter")
include(":code-interpreter-bom")

// Development-time dependency substitution:
// Allow developing this SDK against local sandbox sources without changing published dependencies.
//
// Usage:
// - Enable: ./gradlew -PuseLocalSandbox=true ...
// - Disable (default): do nothing, uses Maven coordinates declared in dependencies.
val useLocalSandbox = providers.gradleProperty("useLocalSandbox").orNull?.toBoolean() == true
if (useLocalSandbox) {
    includeBuild("../../sandbox/kotlin") {
        dependencySubstitution {
            substitute(module("com.alibaba.opensandbox:sandbox")).using(project(":sandbox"))
            substitute(module("com.alibaba.opensandbox:sandbox-api")).using(project(":sandbox-api"))
            substitute(module("com.alibaba.opensandbox:sandbox-bom")).using(project(":sandbox-bom"))
        }
    }
}
