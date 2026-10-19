plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release credentials are supplied by the maintainer/CI, never a generated debug key.
val releaseSigning = listOf(
    "ANDROID_KEYSTORE_PATH", "ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD"
).associateWith { providers.environmentVariable(it).orNull }

android {
    namespace = "ca.cerberusgames.douchesync"
    compileSdk = 36
    defaultConfig {
        applicationId = "ca.cerberusgames.douchesync"
        minSdk = 24
        targetSdk = 36
        val release = file("../../engine/VERSION").readText().trim()
        val parts = release.split(".").map(String::toInt)
        versionCode = parts[0] * 1_000_000 + parts[1] * 1_000 + parts[2]
        versionName = release
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets.getByName("main") {
        java.srcDir("../../.build/fyne-java")
        jniLibs.srcDir("../../.build/fyne-jni")
        assets.srcDir("../../.build/fyne-assets")
    }
    signingConfigs {
        create("release") {
            releaseSigning["ANDROID_KEYSTORE_PATH"]?.let { storeFile = file(it) }
            storePassword = releaseSigning["ANDROID_KEYSTORE_PASSWORD"]
            keyAlias = releaseSigning["ANDROID_KEY_ALIAS"]
            keyPassword = releaseSigning["ANDROID_KEY_PASSWORD"]
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
    }
    testOptions { unitTests.isIncludeAndroidResources = true }
}

kotlin { compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) } }

val checkReleaseSigning by tasks.registering {
    doLast {
        val missing = releaseSigning.filterValues { it.isNullOrBlank() }.keys
        check(missing.isEmpty()) {
            "Release signing is required. Missing: ${missing.joinToString()}. See docs/ANDROID-SIGNING.md."
        }
        check(file(releaseSigning.getValue("ANDROID_KEYSTORE_PATH")!!).isFile) {
            "Release keystore does not exist. See docs/ANDROID-SIGNING.md."
        }
    }
}
tasks.matching { it.name == "preReleaseBuild" }.configureEach { dependsOn(checkReleaseSigning) }

val buildGoCore by
    tasks.registering(Exec::class) {
        workingDir(rootProject.projectDir.parentFile)
        commandLine("bash", "scripts/build-android-core.sh")
        inputs.files(
            fileTree("../../engine") {
                include("*.go", "VERSION")
                exclude("*_test.go")
            }
        )
        inputs.files(
            fileTree("../../mobile") {
                include("*.go", "go.mod", "go.sum")
                exclude("*_test.go")
            }
        )
        inputs.files("../../go.mod", "../../go.sum", "../../scripts/build-android-core.sh")
        outputs.file("libs/douchesync.aar")
    }
val buildFyneUi by tasks.registering(Exec::class) {
    workingDir(rootProject.projectDir.parentFile)
    commandLine("bash", "scripts/build-android-ui.sh")
    inputs.files(fileTree("../../phoneui") { include("*.go", "*.c", "go.mod", "go.sum"); exclude("*_test.go") })
    inputs.file("../../scripts/build-android-ui.sh")
    outputs.dir("../../.build/fyne-java")
    outputs.dir("../../.build/fyne-jni")
    outputs.dir("../../.build/fyne-assets")
}
tasks.named("preBuild") { dependsOn(buildFyneUi) }

val goCore = files("libs/douchesync.aar").builtBy(buildGoCore)

dependencies {
    implementation(goCore)
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("androidx.work:work-runtime-ktx:2.11.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.robolectric:robolectric:4.16.1")
    androidTestImplementation("androidx.test.uiautomator:uiautomator:2.3.0")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
}
