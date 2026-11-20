plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "dev.ezcd.clarity"
    compileSdk = 34

    defaultConfig {
        applicationId = "dev.ezcd.clarity"
        minSdk = 24
        targetSdk = 34
        // CI passes the real values, derived from the release tag (see
        // mobile-assets.yml): versionName is the semver, versionCode is
        // major·10⁶ + minor·10³ + patch so it rises with every release and
        // Android accepts each APK as an update to the last. A local build
        // without the properties is versionCode 1, which no release will ever
        // be again — so a dev install always offers to "update" to a real one.
        versionCode = (project.findProperty("clarityVersionCode") as String?)?.toInt() ?: 1
        versionName = (project.findProperty("clarityVersionName") as String?) ?: "0.0.0-dev"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // The bound core is ~9 MB per ABI, so an APK carrying all four is mostly
        // native code for a device nobody is holding. -PclarityAbi=arm64-v8a
        // builds one you can actually send someone.
        (project.findProperty("clarityAbi") as String?)?.let { abi ->
            ndk { abiFilters += abi.split(",") }
        }
    }

    // The release key lives only in CI (and in Steven's backup): the keystore
    // is written to disk by the workflow and named through these variables.
    // Without them — every local build — release falls back to the debug key,
    // which keeps `assembleRelease` working on a laptop while making it
    // impossible to produce something that installs AS an update to the
    // published app by accident. Android enforces signature continuity, so
    // which key signed the APK is the whole difference between "update" and
    // "please uninstall first".
    val releaseStore = System.getenv("CLARITY_KEYSTORE_FILE")
    signingConfigs {
        if (releaseStore != null) {
            create("release") {
                storeFile = file(releaseStore)
                storePassword = System.getenv("CLARITY_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("CLARITY_KEY_ALIAS")
                keyPassword = System.getenv("CLARITY_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        // Debug is what you build while working; release is what you install on
        // a real phone. Unshrunk, the app is 22 MB of Compose dex nobody calls.
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = if (releaseStore != null) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }

    buildFeatures { compose = true }

    // The Go archive is a build product, not a source file: scripts/bind-android.sh
    // writes it here. Committing 30-odd MB of per-ABI native code would make
    // every bind show up as a binary diff.
    sourceSets["main"].jniLibs.srcDirs("libs")

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }
}

dependencies {
    implementation(files("libs/clarity.aar"))

    // The lite runtime: the full one carries descriptors and reflection this
    // app never uses, and a phone pays for them in both size and startup.
    implementation("com.google.protobuf:protobuf-javalite:3.25.5")

    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.6")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.6")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.6")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation(platform("androidx.compose:compose-bom:2024.10.00"))
    // The extended set, because the chrome needs key, fingerprint, terminal,
    // alt_route and add_link, none of which are in core. R8 strips the rest:
    // each icon is its own lazily-built property, so nothing unreferenced
    // survives into the release build.
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.material3:material3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    // The sanctioned way to run work the user is not watching. Its periodic
    // floor is fifteen minutes and Doze can stretch that, which is the price of
    // not holding a wake lock or a permanent notification.
    implementation("androidx.work:work-runtime-ktx:2.9.1")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")

    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
}
