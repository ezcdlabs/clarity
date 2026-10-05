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
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // The bound core is ~9 MB per ABI, so an APK carrying all four is mostly
        // native code for a device nobody is holding. -PclarityAbi=arm64-v8a
        // builds one you can actually send someone.
        (project.findProperty("clarityAbi") as String?)?.let { abi ->
            ndk { abiFilters += abi.split(",") }
        }
    }

    buildTypes {
        // Debug is what you build while working; release is what you install on
        // a real phone. Unshrunk, the app is 22 MB of Compose dex nobody calls.
        //
        // It signs with the debug key: there is no upload key, nothing is
        // published, and the point of this build type is a side-loadable APK,
        // not a shippable one.
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.getByName("debug")
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

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")

    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
}
