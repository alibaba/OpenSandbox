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

@file:Suppress("UnstableApiUsage")

import org.jetbrains.kotlin.gradle.dsl.KotlinJvmProjectExtension

buildscript {
    repositories {
        mavenCentral()
        gradlePluginPortal()
    }

    dependencies {
        classpath(libs.bundles.jackson.build)
    }
}

plugins {
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.dokka) apply false
    alias(libs.plugins.spotless)
    alias(libs.plugins.mavenPublish) apply false
}

allprojects {
    group = project.findProperty("project.group") as String
    version = project.findProperty("project.version") as String

    repositories {
        mavenCentral()
    }
}

configure<com.diffplug.gradle.spotless.SpotlessExtension> {
    kotlin {
        target("**/*.kt")
        targetExclude("**/build/**/*.kt", "**/bin/**/*.kt", "**/generated/**/*.kt")
        ktlint()
    }
    kotlinGradle {
        target("**/*.gradle.kts")
        ktlint()
    }
}

val kotlinJvmId = libs.plugins.kotlin.jvm.get().pluginId
val kotlinSerializationId = libs.plugins.kotlin.serialization.get().pluginId
val dokkaId = libs.plugins.dokka.get().pluginId
val mavenPublishId = libs.plugins.mavenPublish.get().pluginId

subprojects {
    apply(plugin = mavenPublishId)
    if (name != "code-interpreter-bom") {
        apply(plugin = kotlinJvmId)
        apply(plugin = kotlinSerializationId)
        apply(plugin = dokkaId)

        configure<KotlinJvmProjectExtension> {
            jvmToolchain(8)
            compilerOptions {
                javaParameters.set(true)
                freeCompilerArgs.add("-Xjvm-default=all")
            }
        }
    }

    // Include license file in published artifacts (jars/sources jars) for compliance and clarity.
    tasks.withType<Jar>().configureEach {
        from(rootProject.file("LICENSE")) {
            into("META-INF")
        }
    }

    configure<com.vanniktech.maven.publish.MavenPublishBaseExtension> {
        coordinates(project.group.toString(), project.name, project.version.toString())
        publishToMavenCentral()
        signAllPublications()
        pom {
            name.set(project.name)
            description.set("Alibaba Code Interpreter SDK")
            inceptionYear.set("2025")
            url.set("https://github.com/alibaba/OpenSandbox")
            licenses {
                license {
                    name.set("The Apache License, Version 2.0")
                    url.set("https://www.apache.org/licenses/LICENSE-2.0.txt")
                    distribution.set("repo")
                }
            }
            developers {
                developer {
                    id.set("alibaba")
                    name.set("Alibaba Group")
                    url.set("https://github.com/alibaba")
                    email.set("ninan.nn@alibaba-inc.com")
                }
            }
            scm {
                url.set("https://github.com/alibaba/OpenSandbox")
                connection.set("scm:git:https://github.com/alibaba/OpenSandbox.git")
                developerConnection.set("scm:git:ssh://git@github.com/alibaba/OpenSandbox.git")
            }
        }
    }
}
