<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('idle_shutdown_settings', function (Blueprint $table) {
            $table->boolean('wake_enabled')->default(false);
            $table->json('wake_ports')->nullable();
            $table->string('wake_match_prefix')->nullable();
            $table->unsignedInteger('wake_match_length')->nullable();
            $table->timestamp('last_woken_at')->nullable();
        });

        Schema::create('lazy_pelican_agents', function (Blueprint $table) {
            $table->increments('id');
            $table->string('agent_key')->unique();
            $table->unsignedInteger('user_id');
            $table->string('node_key')->nullable();
            $table->string('hostname');
            $table->string('version');
            $table->string('interface')->nullable();
            $table->boolean('capture_ok')->default(false);
            $table->text('capture_error')->nullable();
            $table->json('watching')->nullable();
            $table->timestamp('last_seen_at');
            $table->timestamps();

            $table->foreign('user_id')->references('id')->on('users')->cascadeOnDelete();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('lazy_pelican_agents');

        Schema::table('idle_shutdown_settings', function (Blueprint $table) {
            $table->dropColumn(['wake_enabled', 'wake_ports', 'wake_match_prefix', 'wake_match_length', 'last_woken_at']);
        });
    }
};
