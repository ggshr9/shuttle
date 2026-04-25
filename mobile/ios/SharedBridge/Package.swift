// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SharedBridge",
    platforms: [.iOS(.v15)],
    products: [
        .library(name: "SharedBridge", targets: ["SharedBridge"]),
    ],
    targets: [
        .target(name: "SharedBridge", dependencies: []),
        .testTarget(name: "SharedBridgeTests", dependencies: ["SharedBridge"]),
    ]
)
