plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release signing comes only from the environment, never from a file in the
// repository: NUDGE_KEYSTORE (path to the .jks), NUDGE_KEYSTORE_PASSWORD,
// NUDGE_KEY_ALIAS and NUDGE_KEY_PASSWORD. Without NUDGE_KEYSTORE the release
// build is left unsigned (it never falls back to the debug key).
val releaseKeystore: String? = System.getenv("NUDGE_KEYSTORE")?.takeIf { it.isNotBlank() }

android {
    // Kotlin package of the sources; the installed app id is applicationId.
    namespace = "com.clipboardshare"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.scag95.nudge"
        minSdk = 24
        targetSdk = 34
        versionCode = 2
        versionName = "1.0.0"
    }

    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = file(releaseKeystore)
                storePassword = System.getenv("NUDGE_KEYSTORE_PASSWORD")
                    ?: error("NUDGE_KEYSTORE is set but NUDGE_KEYSTORE_PASSWORD is not")
                keyAlias = System.getenv("NUDGE_KEY_ALIAS") ?: "nudge"
                keyPassword = System.getenv("NUDGE_KEY_PASSWORD")
                    ?: System.getenv("NUDGE_KEYSTORE_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            if (releaseKeystore != null) {
                signingConfig = signingConfigs.getByName("release")
            }
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    testImplementation("junit:junit:4.13.2")
}
