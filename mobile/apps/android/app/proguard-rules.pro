# gomobile's generated classes are reached from native code by name, and the
# JNI entry points are never called from Kotlin at all — R8 cannot see any of
# it and strips the lot without this.
-keep class go.** { *; }
-keep class dev.ezcd.clarity.core.** { *; }
-keepclasseswithmembernames class * { native <methods>; }

# protobuf-javalite builds its message schemas by reflecting over the generated
# classes' fields.
-keep class * extends com.google.protobuf.GeneratedMessageLite { *; }
-keepclassmembers class * extends com.google.protobuf.GeneratedMessageLite { <fields>; }
-dontwarn com.google.protobuf.**
